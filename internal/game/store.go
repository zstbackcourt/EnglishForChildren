package game

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrNotFound = errors.New("没有找到这个角色或挑战")

type Save struct {
	SchemaVersion int               `json:"schemaVersion"`
	Players       []*Player         `json:"players"`
	Results       map[string]Result `json:"results"`
}
type Store struct {
	mu     sync.Mutex
	path   string
	state  Save
	memory bool
}

func newID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, state: Save{SchemaVersion: 1, Players: []*Player{}, Results: map[string]Result{}}}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &s.state); e != nil {
		return nil, fmt.Errorf("存档损坏，原文件未修改: %w", e)
	}
	if s.state.SchemaVersion != 1 || s.state.Players == nil || s.state.Results == nil {
		return nil, errors.New("不支持的存档格式")
	}
	seen := map[string]bool{}
	for _, p := range s.state.Players {
		if p == nil || p.ID == "" || seen[p.ID] || p.Progress == nil {
			return nil, errors.New("存档角色数据无效")
		}
		seen[p.ID] = true
		for _, pr := range p.Progress {
			if pr == nil || pr.Levels == nil {
				return nil, errors.New("存档进度数据无效")
			}
			for _, lv := range pr.Levels {
				if lv == nil {
					return nil, errors.New("存档奖励数据无效")
				}
			}
		}
	}
	return s, nil
}
func clone[T any](v T) T { b, _ := json.Marshal(v); var out T; json.Unmarshal(b, &out); return out }
func (s *Store) commit(next Save) error {
	if s.memory {
		s.state = next
		return nil
	}
	if e := os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(s.path), ".save-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), s.path); e != nil {
		return e
	}
	s.state = next
	return nil
}
func findPlayer(s *Save, id string) *Player {
	for _, p := range s.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}
func (s *Store) List() []*Player { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.state.Players) }
func (s *Store) Player(id string) (*Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := findPlayer(&s.state, id)
	if p == nil {
		return nil, ErrNotFound
	}
	return clone(p), nil
}
func (s *Store) Create(name, avatar string) (*Player, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 20 {
		return nil, errors.New("名字需要 1–20 个字符")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return nil, errors.New("名字不能包含控制字符")
		}
	}
	if avatar != "fox" && avatar != "cat" && avatar != "rabbit" && avatar != "bear" {
		return nil, errors.New("请选择一个头像")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.Players) >= 100 {
		return nil, errors.New("家庭角色已达到 100 个")
	}
	next := clone(s.state)
	p := &Player{ID: newID(), Name: name, Avatar: avatar, CreatedAt: now(), UpdatedAt: now(), LastLesson: 1, Progress: map[int]*Progress{}}
	next.Players = append(next.Players, p)
	if e := s.commit(next); e != nil {
		return nil, e
	}
	return clone(p), nil
}
func progress(p *Player, id int) *Progress {
	pr := p.Progress[id]
	if pr == nil {
		pr = &Progress{Levels: map[string]*Level{}, LastDifficulty: "easy"}
		p.Progress[id] = pr
	}
	return pr
}
func (s *Store) SaveProgress(id string, lesson, cursor int, difficulty string) error {
	if lesson < 1 || lesson > 144 || cursor < 0 || cursor > 500 || thresholds[difficulty] == 0 {
		return errors.New("无效的学习进度")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	p := findPlayer(&next, id)
	if p == nil {
		return ErrNotFound
	}
	pr := progress(p, lesson)
	pr.Cursor = cursor
	pr.LastDifficulty = difficulty
	p.LastLesson = lesson
	p.UpdatedAt = now()
	return s.commit(next)
}
func (s *Store) Award(id string, lesson int, difficulty string, score int, attempt string) (Result, error) {
	return s.award(id, lesson, difficulty, score, attempt, nil)
}
func (s *Store) award(id string, lesson int, difficulty string, score int, attempt string, feedback []Feedback) (Result, error) {
	if lesson < 1 || lesson > 144 || thresholds[difficulty] == 0 || score < 0 || score > 5 || attempt == "" {
		return Result{}, errors.New("无效的挑战结果")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := id + ":" + attempt
	if r, ok := s.state.Results[key]; ok {
		return clone(r), nil
	}
	next := clone(s.state)
	p := findPlayer(&next, id)
	if p == nil {
		return Result{}, ErrNotFound
	}
	pr := progress(p, lesson)
	lv := pr.Levels[difficulty]
	if lv == nil {
		lv = &Level{}
		pr.Levels[difficulty] = lv
	}
	lv.Attempts++
	if score > lv.BestScore {
		lv.BestScore = score
	}
	r := Result{Score: score, Total: 5, Passed: score >= thresholds[difficulty], Difficulty: difficulty, LessonID: lesson, Feedback: feedback}
	if r.Passed && !lv.Trophy {
		lv.Trophy = true
		r.StarsEarned = points[difficulty]
		p.Stars += r.StarsEarned
	}
	pr.LastDifficulty = difficulty
	p.LastLesson = lesson
	p.UpdatedAt = now()
	next.Results[key] = r
	if e := s.commit(next); e != nil {
		return Result{}, e
	}
	return clone(r), nil
}
func (s *Store) SavedResult(id, attempt string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.state.Results[id+":"+attempt]
	return clone(r), ok
}
func (s *Store) Backup() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.MarshalIndent(s.state, "", "  ")
	return b
}
