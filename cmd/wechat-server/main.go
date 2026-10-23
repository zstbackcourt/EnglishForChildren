package main

import (
	"context"
	"crypto/rand"
	"englishadventure/internal/game"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func newServer(args []string, getenv func(string) string) (*http.Server, error) {
	flags := flag.NewFlagSet("wechat-server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "project root")
	addr := flags.String("addr", "127.0.0.1:8093", "listen address")
	save := flags.String("save-dir", "data/wechat", "account save directory relative to root or absolute")
	dev := flags.Bool("dev-login", false, "allow local development login on loopback only")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, errors.New("unexpected positional arguments")
	}
	config := game.WeChatConfig{SaveDir: *save, SessionKey: []byte(getenv("WECHAT_SESSION_KEY")), DevLogin: *dev, ListenAddr: *addr}
	if *dev {
		if len(config.SessionKey) == 0 {
			// Local sessions intentionally expire across restarts unless a key is supplied.
			config.SessionKey = make([]byte, 32)
			if _, err := rand.Read(config.SessionKey); err != nil {
				return nil, errors.New("unable to initialize development session key")
			}
		}
	} else {
		appID, appSecret := getenv("WECHAT_APP_ID"), getenv("WECHAT_APP_SECRET")
		if strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" {
			return nil, errors.New("WECHAT_APP_ID and WECHAT_APP_SECRET are required")
		}
		config.Exchanger = game.WeChatAPIExchanger{AppID: appID, AppSecret: appSecret}
	}
	if len(config.SessionKey) < 32 {
		return nil, errors.New("WECHAT_SESSION_KEY must contain at least 32 bytes")
	}
	storage := getenv("WECHAT_STORAGE")
	if storage != "" && storage != "file" && storage != "mysql" {
		return nil, errors.New("invalid WECHAT_STORAGE")
	}
	if storage == "mysql" {
		backend, err := game.OpenMySQLAccounts(context.Background(), getenv("MYSQL_ADDRESS"), getenv("MYSQL_USERNAME"), getenv("MYSQL_PASSWORD"), getenv("MYSQL_DATABASE"))
		if err != nil {
			return nil, err
		}
		config.Backend = backend
	}
	var initialized bool
	defer func() {
		if !initialized && config.Backend != nil {
			config.Backend.(*game.MySQLAccounts).Close()
		}
	}()
	abs, err := filepath.Abs(*root)
	if err != nil {
		return nil, errors.New("invalid project root")
	}
	catalog, err := game.LoadCatalog(abs)
	if err != nil {
		return nil, err
	}
	handler, err := game.NewWeChatHandler(abs, catalog, config)
	if err != nil {
		return nil, err
	}
	initialized = true
	server := &http.Server{
		Addr: *addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	if config.Backend != nil {
		server.RegisterOnShutdown(func() { config.Backend.(*game.MySQLAccounts).Close() })
	}
	return server, nil
}

func main() {
	server, err := newServer(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("WeChat English Adventure listening on %s", server.Addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
