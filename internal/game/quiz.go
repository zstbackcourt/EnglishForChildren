package game

import (
	"errors"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

func normalize(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == '’' {
			return '\''
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)), " ")
}

var wordsRE = regexp.MustCompile(`[A-Za-z]+(?:['’][A-Za-z]+)?`)

type Question struct {
	ID      int      `json:"id"`
	Type    string   `json:"type"`
	Prompt  string   `json:"prompt"`
	Display string   `json:"display,omitempty"`
	Options []string `json:"options,omitempty"`
	Start   float64  `json:"start"`
	End     float64  `json:"end"`
	Answer  string   `json:"-"`
	Text    string   `json:"-"`
}
type Attempt struct {
	ID         string     `json:"id"`
	PlayerID   string     `json:"-"`
	LessonID   int        `json:"lessonId"`
	Difficulty string     `json:"difficulty"`
	Questions  []Question `json:"questions"`
	Created    time.Time  `json:"-"`
}
type Quizzes struct {
	mu    sync.Mutex
	items map[string]*Attempt
}

func newQuizzes() *Quizzes { return &Quizzes{items: map[string]*Attempt{}} }
func (q *Quizzes) Start(player string, l Lesson, d string) (*Attempt, error) {
	if thresholds[d] == 0 {
		return nil, errors.New("请选择有效难度")
	}
	lines := []Line{}
	seen := map[string]bool{}
	for _, line := range l.Lines {
		key := normalize(line.Text)
		if len(wordsRE.FindAllString(line.Text, -1)) >= 1 && !seen[key] {
			lines = append(lines, line)
			seen[key] = true
		}
	}
	if len(lines) == 0 {
		return nil, errors.New("本课没有可生成题目的语句")
	}
	rand.Shuffle(len(lines), func(i, j int) { lines[i], lines[j] = lines[j], lines[i] })
	a := &Attempt{ID: newID(), PlayerID: player, LessonID: l.ID, Difficulty: d, Created: time.Now()}
	for i := 0; i < 5; i++ {
		ln := lines[i%len(lines)]
		w := wordsRE.FindAllString(ln.Text, -1)
		target := w[0]
		for _, x := range w {
			if len(x) > len(target) {
				target = x
			}
		}
		it := Question{ID: i, Start: ln.Start, End: ln.End, Text: ln.Text}
		switch {
		case d == "easy":
			it.Type = "choice"
			it.Prompt = "听一听，选出你听到的句子"
			it.Answer = ln.Text
			it.Options = []string{ln.Text}
			for _, other := range l.Lines {
				if normalize(other.Text) != normalize(ln.Text) && len(it.Options) < 3 {
					duplicate := false
					for _, o := range it.Options {
						if normalize(o) == normalize(other.Text) {
							duplicate = true
						}
					}
					if !duplicate {
						it.Options = append(it.Options, other.Text)
					}
				}
			}
		case d == "medium" && i%2 == 1 && len(w) >= 2 && len(w) <= 14:
			it.Type = "order"
			it.Prompt = "听录音，把单词排成一句话"
			it.Answer = strings.Join(w, " ")
			it.Options = append([]string{}, w...)
		default:
			it.Type = "fill"
			it.Prompt = "听录音，写出空缺的单词"
			it.Display = strings.Replace(ln.Text, target, "____", 1)
			it.Answer = target
			if d == "medium" {
				it.Type = "choice"
				it.Options = []string{target}
				for _, other := range l.Lines {
					for _, v := range wordsRE.FindAllString(other.Text, -1) {
						dup := false
						for _, o := range it.Options {
							if normalize(o) == normalize(v) {
								dup = true
							}
						}
						if !dup && len(v) > 2 && len(it.Options) < 4 {
							it.Options = append(it.Options, v)
						}
					}
				}
			}
		}
		rand.Shuffle(len(it.Options), func(i, j int) { it.Options[i], it.Options[j] = it.Options[j], it.Options[i] })
		a.Questions = append(a.Questions, it)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, old := range q.items {
		if time.Since(old.Created) > 2*time.Hour {
			delete(q.items, id)
		}
	}
	// Bound memory while retaining a family's most recent open challenges.
	if len(q.items) >= 500 {
		ids := make([]string, 0, len(q.items))
		for id := range q.items {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return q.items[ids[i]].Created.Before(q.items[ids[j]].Created) })
		delete(q.items, ids[0])
	}
	q.items[a.ID] = a
	return a, nil
}
func (q *Quizzes) Submit(s *Store, player, id string, answers []string) (Result, error) {
	if r, ok := s.SavedResult(player, id); ok {
		return r, nil
	}
	q.mu.Lock()
	a := q.items[id]
	q.mu.Unlock()
	if a == nil || a.PlayerID != player {
		return Result{}, ErrNotFound
	}
	if time.Since(a.Created) > 2*time.Hour {
		return Result{}, errors.New("挑战已过期，请重新开始")
	}
	if len(answers) != len(a.Questions) {
		return Result{}, errors.New("请完成全部 5 道题")
	}
	score := 0
	feedback := []Feedback{}
	for i, question := range a.Questions {
		ok := normalize(answers[i]) == normalize(question.Answer)
		if ok {
			score++
		}
		feedback = append(feedback, Feedback{Correct: ok, Expected: question.Answer, Text: question.Text})
	}
	return s.award(player, a.LessonID, a.Difficulty, score, id, feedback)
}
