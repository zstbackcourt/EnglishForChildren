package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsSeparatePlayers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Create(" 小明 ", "fox")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create("小明", "cat")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.Name != "小明" {
		t.Fatal("identity or name")
	}
	if err = s.SaveProgress(a.ID, 1, 3, "easy"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Award(a.ID, 1, "easy", 3, "once")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Award(a.ID, 1, "easy", 3, "once")
	if err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = s.Player(a.ID)
	b, _ = s.Player(b.ID)
	if a.Stars != 10 || a.Progress[1].Cursor != 3 || a.Progress[1].Levels["easy"].Attempts != 1 {
		t.Fatalf("bad restored progress %+v", a)
	}
	if b.Stars != 0 || len(b.Progress) != 0 {
		t.Fatal("cross player leak")
	}
}
func TestDifficultyThresholdAndRewards(t *testing.T) {
	for _, tc := range []struct {
		d           string
		score, want int
	}{{"easy", 2, 0}, {"easy", 3, 10}, {"medium", 3, 0}, {"medium", 4, 20}, {"hard", 4, 0}, {"hard", 5, 30}} {
		s, _ := OpenStore(filepath.Join(t.TempDir(), "save.json"))
		p, _ := s.Create("Amy", "fox")
		r, e := s.Award(p.ID, 1, tc.d, tc.score, "one")
		if e != nil {
			t.Fatal(e)
		}
		if r.StarsEarned != tc.want {
			t.Fatalf("%+v got %+v", tc, r)
		}
		r, e = s.Award(p.ID, 1, tc.d, tc.score, "two")
		if e != nil || r.StarsEarned != 0 {
			t.Fatal("repeat award")
		}
	}
}
func TestInvalidInputAndCorruptStore(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "save.json"))
	if _, e := s.Create("  ", "fox"); e == nil {
		t.Fatal("empty name accepted")
	}
	if _, e := s.Create("Amy", "invalid"); e == nil {
		t.Fatal("invalid avatar")
	}
	p, _ := s.Create("Amy", "fox")
	if _, e := s.Award(p.ID, 1, "evil", 5, "x"); e == nil {
		t.Fatal("invalid difficulty")
	}
	if _, e := s.Award(p.ID, 1, "easy", 6, "x"); e == nil {
		t.Fatal("invalid score")
	}
	if e := s.SaveProgress("missing", 1, 0, "easy"); e == nil {
		t.Fatal("missing player")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, e := OpenStore(path); e == nil {
		t.Fatal("silently overwrote corrupt save")
	}
}
func TestFailedDiskWriteDoesNotCommitMemory(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(filepath.Join(dir, "save.json"))
	p, _ := s.Create("Amy", "fox")
	os.Remove(s.path)
	os.Mkdir(s.path, 0700)
	if _, e := s.Award(p.ID, 1, "easy", 5, "x"); e == nil {
		t.Fatal("expected disk error")
	}
	p, _ = s.Player(p.ID)
	if p.Stars != 0 {
		t.Fatal("memory committed after disk failure")
	}
}
func TestAnswerNormalization(t *testing.T) {
	if normalize("  Hello, WORLD! ") != "hello world" {
		t.Fatal("punctuation/case")
	}
	if normalize("isn't") == normalize("isnt") {
		t.Fatal("contraction apostrophe lost")
	}
}
