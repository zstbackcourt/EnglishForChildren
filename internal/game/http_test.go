package game

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Catalog, *Store, string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "web"), 0700)
	os.WriteFile(filepath.Join(root, "a.mp3"), []byte("0123456789"), 0600)
	s, _ := OpenStore(filepath.Join(root, "save.json"))
	c := &Catalog{Lessons: []Lesson{{ID: 1, AudioFile: "a.mp3", Lines: []Line{{Start: 1, End: 2, Text: "Excuse me!"}, {Start: 2, End: 3, Text: "Is this your handbag?"}, {Start: 3, End: 4, Text: "Yes it is."}, {Start: 4, End: 5, Text: "Thank you very much."}}}}}
	return c, s, root
}
func TestAPIRejectsInvalidProgressAndSupportsAudioRanges(t *testing.T) {
	c, s, root := fixture(t)
	p, _ := s.Create("Amy", "fox")
	h := NewHandler(root, c, s)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/players/"+p.ID+"/progress", strings.NewReader(`{"lessonId":1,"cursor":99,"difficulty":"easy"}`))
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid cursor: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/media/audio/1", nil)
	r.Header.Set("Range", "bytes=2-5")
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "2345" {
		t.Fatalf("range %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/media/audio/999", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/lessons/1", nil))
	if strings.Contains(w.Body.String(), "audioFile") {
		t.Fatal("local path leaked")
	}
}
func TestQuizGradesServerSideAndRejectsOtherPlayer(t *testing.T) {
	c, s, _ := fixture(t)
	a, _ := s.Create("Amy", "fox")
	b, _ := s.Create("Ben", "cat")
	q := newQuizzes()
	attempt, e := q.Start(a.ID, c.Lessons[0], "hard")
	if e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(attempt)
	if strings.Contains(string(payload), `"Answer"`) || strings.Contains(string(payload), `"answer"`) {
		t.Fatal("answer leaked")
	}
	answers := make([]string, 5)
	for i, x := range attempt.Questions {
		answers[i] = strings.ToUpper(x.Answer) + "!"
	}
	if _, e = q.Submit(s, b.ID, attempt.ID, answers); e == nil {
		t.Fatal("cross player submit")
	}
	r, e := q.Submit(s, a.ID, attempt.ID, answers)
	if e != nil || r.Score != 5 || r.StarsEarned != 30 {
		t.Fatalf("%+v %v", r, e)
	}
	q.Submit(s, a.ID, attempt.ID, answers)
	a, _ = s.Player(a.ID)
	if a.Stars != 30 || a.Progress[1].Levels["hard"].Attempts != 1 {
		t.Fatal("duplicate submit")
	}
}
func TestCrossOriginWriteRejected(t *testing.T) {
	c, s, root := fixture(t)
	h := NewHandler(root, c, s)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/players", strings.NewReader(`{"name":"Amy","avatar":"fox"}`))
	r.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestQuizAvoidsRepeatedDialogueWhenEnoughUniqueLines(t *testing.T) {
	l := Lesson{ID: 1, Lines: []Line{{Text: "Excuse me!"}, {Text: "Yes?"}, {Text: "Is this your handbag?"}, {Text: "Pardon?"}, {Text: "Is this your handbag?"}, {Text: "Yes it is."}, {Text: "Thank you very much."}}}
	for _, d := range []string{"easy", "medium", "hard"} {
		a, e := newQuizzes().Start("p", l, d)
		if e != nil {
			t.Fatal(e)
		}
		seen := map[string]bool{}
		for _, q := range a.Questions {
			if seen[q.Text] {
				t.Fatalf("%s repeated: %s", d, q.Text)
			}
			seen[q.Text] = true
		}
	}
}
func TestEveryLessonCanGenerateAllDifficulties(t *testing.T) {
	root := filepath.Join("..", "..")
	c, e := LoadCatalog(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, l := range c.Lessons {
		for _, d := range []string{"easy", "medium", "hard"} {
			a, e := newQuizzes().Start("p", l, d)
			if e != nil {
				t.Fatalf("lesson %d: %v", l.ID, e)
			}
			if len(a.Questions) != 5 {
				t.Fatal("question count")
			}
			for _, q := range a.Questions {
				if q.Answer == "" || q.Start < 0 || q.End != 0 && q.End <= q.Start {
					t.Fatalf("invalid question: %d %+v", l.ID, q)
				}
				if q.Type == "choice" && len(q.Options) < 2 {
					t.Fatalf("no distractors: %d", l.ID)
				}
			}
		}
	}
}
