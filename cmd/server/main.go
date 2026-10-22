package main

import (
	"englishadventure/internal/game"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	defaultAddr := "127.0.0.1:8080"
	if port := os.Getenv("PORT"); port != "" {
		defaultAddr = "0.0.0.0:" + port
	}
	root := flag.String("root", ".", "project root")
	addr := flag.String("addr", defaultAddr, "listen address")
	save := flag.String("save", "data/save.json", "save file relative to root or absolute")
	flag.Parse()
	abs, e := filepath.Abs(*root)
	if e != nil {
		log.Fatal(e)
	}
	c, e := game.LoadCatalog(abs)
	if e != nil {
		log.Fatal(e)
	}
	path := *save
	if !filepath.IsAbs(path) {
		path = filepath.Join(abs, path)
	}
	s, e := game.OpenStore(path)
	if e != nil {
		log.Fatal(e)
	}
	handler := game.FamilyAccess(game.NewHandler(abs, c, s), os.Getenv("GAME_PASSWORD"))
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("English Adventure: http://%s (%d lessons)", *addr, len(c.Lessons))
	log.Fatal(srv.ListenAndServe())
}
