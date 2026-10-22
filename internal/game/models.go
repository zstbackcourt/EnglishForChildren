package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Line struct {
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Text        string  `json:"text"`
	Speaker     string  `json:"speaker,omitempty"`
	Translation string  `json:"translation,omitempty"`
}
type Lesson struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	PDFPage     int    `json:"pdfPage"`
	AudioLesson int    `json:"audioLesson"`
	AudioFile   string `json:"audioFile,omitempty"`
	AudioURL    string `json:"audioUrl"`
	PageImage   string `json:"pageImage"`
	Scene       string `json:"scene,omitempty"`
	Lines       []Line `json:"lines"`
}
type Catalog struct {
	Version int      `json:"version"`
	Lessons []Lesson `json:"lessons"`
}

func LoadCatalog(root string) (*Catalog, error) {
	b, e := os.ReadFile(filepath.Join(root, "content/book1.json"))
	if e != nil {
		return nil, e
	}
	var c Catalog
	if e = json.Unmarshal(b, &c); e != nil {
		return nil, e
	}
	if len(c.Lessons) != 144 {
		return nil, fmt.Errorf("expected 144 lessons, got %d", len(c.Lessons))
	}
	for i, l := range c.Lessons {
		if l.ID != i+1 || len(l.Lines) < 3 {
			return nil, fmt.Errorf("invalid lesson %d", l.ID)
		}
		for _, f := range []string{filepath.Join(root, l.AudioFile), filepath.Join(root, "web", l.PageImage)} {
			if _, e = os.Stat(f); e != nil {
				return nil, e
			}
		}
	}
	return &c, nil
}
func (c *Catalog) Lesson(id int) (Lesson, bool) {
	if id < 1 || id > len(c.Lessons) {
		return Lesson{}, false
	}
	return c.Lessons[id-1], true
}

type Level struct {
	BestScore int  `json:"bestScore"`
	Attempts  int  `json:"attempts"`
	Trophy    bool `json:"trophy"`
}
type Progress struct {
	Cursor         int               `json:"cursor"`
	LastDifficulty string            `json:"lastDifficulty"`
	Levels         map[string]*Level `json:"levels"`
}
type Player struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Avatar     string            `json:"avatar"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
	Stars      int               `json:"stars"`
	LastLesson int               `json:"lastLesson"`
	Progress   map[int]*Progress `json:"progress"`
}
type Result struct {
	Score       int        `json:"score"`
	Total       int        `json:"total"`
	Passed      bool       `json:"passed"`
	StarsEarned int        `json:"starsEarned"`
	Difficulty  string     `json:"difficulty"`
	LessonID    int        `json:"lessonId"`
	Feedback    []Feedback `json:"feedback,omitempty"`
}
type Feedback struct {
	Correct  bool   `json:"correct"`
	Expected string `json:"expected"`
	Text     string `json:"text"`
}

var thresholds = map[string]int{"easy": 3, "medium": 4, "hard": 5}
var points = map[string]int{"easy": 10, "medium": 20, "hard": 30}
