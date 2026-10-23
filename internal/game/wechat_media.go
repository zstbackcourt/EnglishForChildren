package game

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// confinedMedia opens only regular files below the resolved public media root.
// The root may itself be a configured asset symlink; descendants cannot escape it.
func confinedMedia(root, name string) (*os.File, os.FileInfo, error) {
	if !filepath.IsLocal(name) {
		return nil, nil, os.ErrNotExist
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(base, name))
	if err != nil {
		return nil, nil, err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, nil, os.ErrNotExist
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, info, nil
}

func (g *wechatGateway) media(w http.ResponseWriter, r *http.Request) bool {
	var base, name string
	switch {
	case strings.HasPrefix(r.URL.Path, "/assets/scenes/"):
		base, name = filepath.Join(g.root, "web", "assets", "scenes"), strings.TrimPrefix(r.URL.Path, "/assets/scenes/")
	case strings.HasPrefix(r.URL.Path, "/assets/pages/"):
		base, name = filepath.Join(g.root, "web", "assets", "pages"), strings.TrimPrefix(r.URL.Path, "/assets/pages/")
	case strings.HasPrefix(r.URL.Path, "/media/audio/"):
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/media/audio/"))
		lesson, ok := g.catalog.Lesson(id)
		if err != nil || !ok || id%2 == 0 || filepath.Ext(lesson.AudioFile) != ".mp3" || !filepath.IsLocal(lesson.AudioFile) {
			http.NotFound(w, r)
			return true
		}
		base, name = g.root, lesson.AudioFile
		// The repository's course archive can be a symlink to the existing asset source.
		if strings.HasPrefix(name, "New-Concept-English/") {
			base, name = filepath.Join(g.root, "New-Concept-English"), strings.TrimPrefix(name, "New-Concept-English/")
		}
	default:
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		reply(w, http.StatusMethodNotAllowed, map[string]string{"error": "素材只允许读取"})
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".png", ".jpg", ".jpeg", ".webp":
		default:
			http.NotFound(w, r)
			return true
		}
	}
	f, info, err := confinedMedia(base, name)
	if err != nil {
		http.NotFound(w, r)
		return true
	}
	defer f.Close()
	if strings.HasPrefix(r.URL.Path, "/media/audio/") {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return true
}
