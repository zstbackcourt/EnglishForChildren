package game

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
)

const maxAccountBytes = (16 << 20) - 1
const maxResponseBytes = 16 << 20

// MySQLAccountBackend runs apply against the current account state under an
// exclusive transaction lock. It must persist the returned state atomically.
type MySQLAccountBackend interface {
	Update(context.Context, string, func([]byte) ([]byte, error)) error
	Ready(context.Context) error
}

type MySQLAccounts struct{ db *sql.DB }

// OpenMySQLAccounts creates only this application's table in an existing database.
func OpenMySQLAccounts(ctx context.Context, address, user, password, database string) (*MySQLAccounts, error) {
	if address == "" || user == "" || password == "" {
		return nil, errors.New("MySQL configuration is incomplete")
	}
	if database == "" {
		database = "english_adventure"
	}
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = address
	cfg.User = user
	cfg.Passwd = password
	cfg.DBName = database
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, errors.New("MySQL initialization failed")
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS english_wechat_accounts (account_id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY, state MEDIUMBLOB NOT NULL) ENGINE=InnoDB`); err != nil {
		db.Close()
		return nil, errors.New("MySQL initialization failed")
	}
	return &MySQLAccounts{db: db}, nil
}
func (m *MySQLAccounts) Close() error { return m.db.Close() }
func (m *MySQLAccounts) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return m.db.PingContext(ctx)
}
func (m *MySQLAccounts) Update(ctx context.Context, id string, apply func([]byte) ([]byte, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO english_wechat_accounts (account_id,state) VALUES (?,?) ON DUPLICATE KEY UPDATE account_id=account_id`, id, []byte{}); err != nil {
		return err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT state FROM english_wechat_accounts WHERE account_id=? FOR UPDATE`, id).Scan(&raw); err != nil {
		return err
	}
	if len(raw) > maxAccountBytes {
		return errors.New("account too large")
	}
	next, err := apply(raw)
	if err != nil {
		return err
	}
	if len(next) > maxAccountBytes {
		return errors.New("account too large")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE english_wechat_accounts SET state=? WHERE account_id=?`, next, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Private fields deliberately use a separate persistence shape. Public API tags
// continue hiding answers, identity and creation time from clients.
type privateQuestion struct {
	Question Question
	Answer   string
	Text     string
}
type privateAttempt struct {
	Attempt   Attempt
	PlayerID  string
	Created   time.Time
	Questions []privateQuestion
}
type accountState struct {
	Version  int
	Save     Save
	Attempts map[string]privateAttempt
}

func encodeAccount(s *Store, q *Quizzes) ([]byte, error) {
	state := accountState{Version: 1, Save: s.state, Attempts: map[string]privateAttempt{}}
	for id, a := range q.items {
		p := privateAttempt{Attempt: *a, PlayerID: a.PlayerID, Created: a.Created}
		p.Attempt.Questions = nil
		for _, question := range a.Questions {
			p.Questions = append(p.Questions, privateQuestion{question, question.Answer, question.Text})
		}
		state.Attempts[id] = p
	}
	raw, err := json.Marshal(state)
	if len(raw) > maxAccountBytes {
		return nil, errors.New("account too large")
	}
	return raw, err
}
func decodeAccount(raw []byte) (*Store, *Quizzes, error) {
	s := &Store{memory: true, state: Save{SchemaVersion: 1, Players: []*Player{}, Results: map[string]Result{}}}
	q := newQuizzes()
	if len(raw) == 0 {
		return s, q, nil
	}
	var state accountState
	if len(raw) > maxAccountBytes || json.Unmarshal(raw, &state) != nil || state.Version != 1 || state.Save.SchemaVersion != 1 || state.Save.Players == nil || state.Save.Results == nil {
		return nil, nil, errors.New("invalid account state")
	}
	s.state = state.Save
	for id, p := range state.Attempts {
		a := p.Attempt
		a.PlayerID = p.PlayerID
		a.Created = p.Created
		a.Questions = nil
		for _, item := range p.Questions {
			question := item.Question
			question.Answer = item.Answer
			question.Text = item.Text
			a.Questions = append(a.Questions, question)
		}
		q.items[id] = &a
	}
	return s, q, nil
}

type boundedResponse struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (b *boundedResponse) Header() http.Header { return b.header }
func (b *boundedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *boundedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	if b.body.Len()+len(p) > maxResponseBytes {
		b.overflow = true
		return 0, errors.New("response too large")
	}
	return b.body.Write(p)
}
func (g *wechatGateway) durableAccount(w http.ResponseWriter, r *http.Request, subject string) {
	b := &boundedResponse{header: make(http.Header)}
	err := g.config.Backend.Update(r.Context(), subject, func(raw []byte) ([]byte, error) {
		s, q, err := decodeAccount(raw)
		if err != nil {
			return nil, err
		}
		newHandlerWithQuizzes(g.root, g.catalog, s, q).ServeHTTP(b, r)
		if b.overflow || b.status >= 500 {
			return nil, errors.New("request failed")
		}
		return encodeAccount(s, q)
	})
	if err != nil {
		reply(w, 503, map[string]string{"error": "存档暂时不可用"})
		return
	}
	for k, v := range b.header {
		w.Header()[k] = v
	}
	if b.status == 0 {
		b.status = 200
	}
	w.WriteHeader(b.status)
	w.Write(b.body.Bytes())
}
