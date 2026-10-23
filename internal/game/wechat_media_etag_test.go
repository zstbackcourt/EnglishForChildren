package game

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

const wechatTestMediaETag = `"84d89877f0d4041efb6bf91a16f0248f2fd573e6af05c19f96bedb9f882f7882"`

func wechatMediaRequest(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestWeChatMediaStrongETagMatchesCompleteContent(t *testing.T) {
	c, _, root := fixture(t)
	for _, dir := range []string{"scenes", "pages"} {
		base := filepath.Join(root, "web", "assets", dir)
		if err := os.MkdirAll(base, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "lesson.png"), []byte("0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	for _, path := range []string{"/media/audio/1", "/assets/scenes/lesson.png", "/assets/pages/lesson.png"} {
		t.Run(path, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				w := wechatMediaRequest(h, method, path, nil)
				if w.Code != http.StatusOK || w.Header().Get("ETag") != wechatTestMediaETag {
					t.Fatalf("%s: status=%d ETag=%q", method, w.Code, w.Header().Get("ETag"))
				}
				if method == http.MethodGet && w.Body.String() != "0123456789" {
					t.Fatalf("hashing changed response body: %q", w.Body.String())
				}
				if method == http.MethodHead && w.Body.Len() != 0 {
					t.Fatalf("HEAD returned body: %q", w.Body.String())
				}
			}
			w := wechatMediaRequest(h, http.MethodGet, path, map[string]string{"Range": "bytes=2-5"})
			if w.Code != http.StatusPartialContent || w.Header().Get("ETag") != wechatTestMediaETag || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" {
				t.Fatalf("Range: status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestWeChatMediaIfRangeRequiresMatchingStrongETag(t *testing.T) {
	c, _, root := fixture(t)
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	for _, tc := range []struct {
		name, etag, body, contentRange string
		status                         int
	}{
		{"matching", wechatTestMediaETag, "2345", "bytes 2-5/10", http.StatusPartialContent},
		{"changed", `"old-content"`, "0123456789", "", http.StatusOK},
		{"weak", "W/" + wechatTestMediaETag, "0123456789", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", map[string]string{"Range": "bytes=2-5", "If-Range": tc.etag})
			if w.Code != tc.status || w.Body.String() != tc.body || w.Header().Get("Content-Range") != tc.contentRange || w.Header().Get("ETag") != wechatTestMediaETag {
				t.Fatalf("If-Range: status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestWeChatMediaIfNoneMatchReturnsNotModified(t *testing.T) {
	c, _, root := fixture(t)
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	w := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", map[string]string{"If-None-Match": wechatTestMediaETag})
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("ETag") != wechatTestMediaETag {
		t.Fatalf("If-None-Match: status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}

func TestWeChatMediaETagInvalidatesChangedFileMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		mtimeChanged  bool
	}{
		{"mtime", "9876543210", true},
		{"size", "01234567890", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, root := fixture(t)
			h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
			path := filepath.Join(root, c.Lessons[0].AudioFile)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			first := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", nil)
			if first.Header().Get("ETag") != wechatTestMediaETag {
				t.Fatalf("initial ETag=%q", first.Header().Get("ETag"))
			}
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			mtime := info.ModTime()
			if tc.mtimeChanged {
				mtime = mtime.Add(time.Second)
			}
			if err := os.Chtimes(path, mtime, mtime); err != nil {
				t.Fatal(err)
			}
			w := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", map[string]string{"Range": "bytes=2-5", "If-Range": first.Header().Get("ETag")})
			if w.Code != http.StatusOK || w.Body.String() != tc.content || w.Header().Get("Content-Range") != "" || w.Header().Get("ETag") == first.Header().Get("ETag") || w.Header().Get("ETag") == "" {
				t.Fatalf("changed media reused validator: status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestWeChatMediaETagDependsOnContentRatherThanMtime(t *testing.T) {
	c, _, root := fixture(t)
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	first := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", nil)
	mtime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, c.Lessons[0].AudioFile), mtime, mtime); err != nil {
		t.Fatal(err)
	}
	w := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", nil)
	if first.Header().Get("ETag") != wechatTestMediaETag || w.Header().Get("ETag") != wechatTestMediaETag {
		t.Fatalf("identical content ETags: before=%q after=%q", first.Header().Get("ETag"), w.Header().Get("ETag"))
	}
}

func TestWeChatMediaConcurrentRangesShareCompleteContentETag(t *testing.T) {
	c, _, root := fixture(t)
	h := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now)
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func(start int) {
			defer workers.Done()
			w := wechatMediaRequest(h, http.MethodGet, "/media/audio/1", map[string]string{"Range": "bytes=" + strconv.Itoa(start) + "-" + strconv.Itoa(start)})
			if w.Code != http.StatusPartialContent || w.Header().Get("ETag") != wechatTestMediaETag || w.Body.String() != strconv.Itoa(start) {
				t.Errorf("concurrent Range: status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		}(i % 10)
	}
	workers.Wait()
}

func TestWeChatMediaETagCacheUsesOpenedFilePaths(t *testing.T) {
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	for _, path := range []string{"/media/audio/1", "/media/audio/01", "/media/audio/+1", "/media/audio/0001"} {
		w := wechatMediaRequest(g, http.MethodGet, path, nil)
		if w.Code != http.StatusOK || w.Header().Get("ETag") != wechatTestMediaETag {
			t.Fatalf("alias %s: status=%d ETag=%q", path, w.Code, w.Header().Get("ETag"))
		}
	}
	for i := 0; i < 50; i++ {
		w := wechatMediaRequest(g, http.MethodGet, "/assets/scenes/missing-"+strconv.Itoa(i)+".png", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("missing file: status=%d", w.Code)
		}
	}
	if len(g.mediaETags) != 1 {
		t.Fatalf("URL aliases or missing files consumed cache entries: %d", len(g.mediaETags))
	}
}

func TestWeChatMediaETagCacheIsBoundedAcrossAssetChanges(t *testing.T) {
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	base := filepath.Join(root, "web", "assets", "scenes")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < wechatMediaETagCacheLimit+2; i++ {
		name := "lesson-" + strconv.Itoa(i) + ".png"
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
		w := wechatMediaRequest(g, http.MethodGet, "/assets/scenes/"+name, nil)
		if w.Code != http.StatusOK || w.Header().Get("ETag") != wechatTestMediaETag {
			t.Fatalf("new asset: status=%d ETag=%q", w.Code, w.Header().Get("ETag"))
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if len(g.mediaETags) > wechatMediaETagCacheLimit {
			t.Fatalf("obsolete asset metadata grew without bound: %d", len(g.mediaETags))
		}
	}
}

func TestWeChatMediaETagCachesHashAndDoesNotCacheReadFailures(t *testing.T) {
	c, _, root := fixture(t)
	g := wechatHandler(t, c, root, filepath.Join(root, "wechat"), time.Now).(*wechatGateway)
	f, info, err := confinedMedia(root, c.Lessons[0].AudioFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.mediaETag(f, info); err == nil || len(g.mediaETags) != 0 {
		t.Fatalf("failed hash was accepted or cached: err=%v entries=%d", err, len(g.mediaETags))
	}
	w := wechatMediaRequest(g, http.MethodGet, "/media/audio/1", nil)
	if w.Code != http.StatusOK || w.Header().Get("ETag") != wechatTestMediaETag {
		t.Fatalf("retry after failed hash: status=%d ETag=%q", w.Code, w.Header().Get("ETag"))
	}
	// The closed handle would fail if unchanged metadata caused another read.
	if etag, err := g.mediaETag(f, info); err != nil || etag != wechatTestMediaETag {
		t.Fatalf("unchanged file was rehashed: ETag=%q err=%v", etag, err)
	}
}
