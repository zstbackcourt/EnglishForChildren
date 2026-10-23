package game

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Keep one validator per opened asset path, with a fixed cap even if the asset
// directory changes over the process lifetime. Request URLs never become keys.
const wechatMediaETagCacheLimit = 1024

type wechatMediaETag struct {
	size    int64
	modTime time.Time
	etag    string
}

func (g *wechatGateway) mediaETag(f *os.File, info os.FileInfo) (string, error) {
	name := f.Name()
	g.mediaMu.Lock()
	cached, ok := g.mediaETags[name]
	g.mediaMu.Unlock()
	if ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.etag, nil
	}

	// Every request owns its file handle. Hash without holding the cache lock so
	// an uncached MP3 cannot block unrelated media requests or account access.
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	etag := `"` + hex.EncodeToString(hash.Sum(nil)) + `"`
	g.mediaMu.Lock()
	defer g.mediaMu.Unlock()
	if g.mediaETags == nil {
		g.mediaETags = make(map[string]wechatMediaETag)
	}
	if _, exists := g.mediaETags[name]; !exists && len(g.mediaETags) >= wechatMediaETagCacheLimit {
		// Eviction order does not affect correctness; evicted files are rehashed.
		for path := range g.mediaETags {
			delete(g.mediaETags, path)
			break
		}
	}
	g.mediaETags[name] = wechatMediaETag{size: info.Size(), modTime: info.ModTime(), etag: etag}
	return etag, nil
}

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
	etag, err := g.mediaETag(f, info)
	if err != nil {
		reply(w, http.StatusInternalServerError, map[string]string{"error": "素材暂时不可用"})
		return true
	}
	w.Header().Set("ETag", etag)
	if strings.HasPrefix(r.URL.Path, "/media/audio/") {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return true
}
