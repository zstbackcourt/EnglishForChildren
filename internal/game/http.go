package game

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
)

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	if errors.Is(e, ErrNotFound) {
		code = 404
	}
	reply(w, code, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		reply(w, 400, map[string]string{"error": "请求内容无效"})
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		reply(w, 400, map[string]string{"error": "请求只能包含一个 JSON 对象"})
		return false
	}
	return true
}
func publicLesson(l Lesson) Lesson { l.AudioFile = ""; return l }
func NewHandler(root string, c *Catalog, s *Store) http.Handler {
	mux := http.NewServeMux()
	q := newQuizzes()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"status": "ok", "lessons": len(c.Lessons)})
	})
	mux.HandleFunc("GET /api/lessons", func(w http.ResponseWriter, r *http.Request) {
		ls := []Lesson{}
		for _, l := range c.Lessons {
			l = publicLesson(l)
			l.Lines = nil
			ls = append(ls, l)
		}
		reply(w, 200, ls)
	})
	mux.HandleFunc("GET /api/lessons/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		l, ok := c.Lesson(id)
		if !ok {
			failure(w, ErrNotFound)
			return
		}
		reply(w, 200, publicLesson(l))
	})
	mux.HandleFunc("GET /api/players", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, s.List()) })
	mux.HandleFunc("POST /api/players", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name   string `json:"name"`
			Avatar string `json:"avatar"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, e := s.Create(body.Name, body.Avatar)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 201, p)
	})
	mux.HandleFunc("GET /api/players/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, e := s.Player(r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, p)
	})
	mux.HandleFunc("PUT /api/players/{id}/progress", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			LessonID   int    `json:"lessonId"`
			Cursor     int    `json:"cursor"`
			Difficulty string `json:"difficulty"`
		}
		if !decode(w, r, &body) {
			return
		}
		l, ok := c.Lesson(body.LessonID)
		if !ok || body.Cursor < 0 || body.Cursor >= len(l.Lines) {
			failure(w, errors.New("课号或对话位置无效"))
			return
		}
		if e := s.SaveProgress(r.PathValue("id"), body.LessonID, body.Cursor, body.Difficulty); e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, map[string]bool{"saved": true})
	})
	mux.HandleFunc("POST /api/players/{id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, e := s.Player(id); e != nil {
			failure(w, e)
			return
		}
		var body struct {
			LessonID   int    `json:"lessonId"`
			Difficulty string `json:"difficulty"`
		}
		if !decode(w, r, &body) {
			return
		}
		l, ok := c.Lesson(body.LessonID)
		if !ok {
			failure(w, ErrNotFound)
			return
		}
		a, e := q.Start(id, l, body.Difficulty)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 201, a)
	})
	mux.HandleFunc("POST /api/players/{id}/attempts/{attempt}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Answers []string `json:"answers"`
		}
		if !decode(w, r, &body) {
			return
		}
		res, e := q.Submit(s, r.PathValue("id"), r.PathValue("attempt"), body.Answers)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, res)
	})
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="english-adventure-save.json"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Write(s.Backup())
	})
	mux.HandleFunc("GET /media/audio/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		l, ok := c.Lesson(id)
		if !ok || id%2 == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, filepath.Join(root, l.AudioFile))
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 404, map[string]string{"error": "没有这个接口"})
	})
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "web"))))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("request panic: %v", v)
				reply(w, 500, map[string]string{"error": "服务暂时不可用，请稍后重试"})
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				reply(w, 403, map[string]string{"error": "仅允许本地页面访问"})
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			reply(w, 403, map[string]string{"error": "不允许跨站访问"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
