package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestMySQLStartupDiagnosticsNeverExposeConnectionValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"authentication", &mysql.MySQLError{Number: 1045, Message: "private-password private-host"}, "server error 1045 (authentication_denied)"},
		{"database", &mysql.MySQLError{Number: 1049, Message: "private-database"}, "server error 1049 (database_missing)"},
		{"permission", &mysql.MySQLError{Number: 1142, Message: "private-user private-table"}, "server error 1142 (permission_denied)"},
		{"unknown server code", &mysql.MySQLError{Number: 9999, Message: "private-secret"}, "server error 9999 (server_error)"},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, "connection_refused"},
		{"dns", &net.DNSError{Name: "private-host", Err: "private-error"}, "dns_resolution_failed"},
		{"timeout", context.DeadlineExceeded, "connection_timeout"},
		{"unknown", errors.New("private-dsn"), "unclassified_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := mysqlInitializationError("create_table", fmt.Errorf("private-wrapper: %w", tc.cause))
			if !strings.Contains(err.Error(), "MySQL initialization failed at create_table") || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private") {
				t.Fatal(err)
			}
		})
	}
}

type memoryAccounts struct {
	mu   sync.Mutex
	data []byte
	fail bool
}

func (m *memoryAccounts) Ready(context.Context) error { return nil }
func (m *memoryAccounts) Update(_ context.Context, _ string, f func([]byte) ([]byte, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, e := f(m.data)
	if e != nil {
		return e
	}
	if m.fail {
		return errors.New("secret DSN")
	}
	m.data = append([]byte(nil), v...)
	return nil
}
func TestDurableAccountRoundTripAndRollback(t *testing.T) {
	b := &memoryAccounts{}
	g := &wechatGateway{catalog: &Catalog{}, config: WeChatConfig{Backend: b}}
	request := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		g.durableAccount(w, httptest.NewRequest(method, "/api/players", strings.NewReader(body)), "account")
		return w
	}
	if w := request("POST", `{"name":"Amy","avatar":"fox"}`); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	g = &wechatGateway{catalog: &Catalog{}, config: WeChatConfig{Backend: b}}
	if w := request("GET", ""); !strings.Contains(w.Body.String(), "Amy") {
		t.Fatal(w.Body.String())
	}
	before := string(b.data)
	b.fail = true
	if w := request("POST", `{"name":"Ben","avatar":"cat"}`); w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
	if string(b.data) != before {
		t.Fatal("persisted failed transaction")
	}
}
func TestPrivateAttemptStateRoundTrip(t *testing.T) {
	q := newQuizzes()
	q.items["a"] = &Attempt{ID: "a", PlayerID: "p", Questions: []Question{{Answer: "secret", Text: "source"}}}
	s := &Store{state: Save{SchemaVersion: 1, Players: []*Player{}, Results: map[string]Result{}}}
	raw, e := encodeAccount(s, q)
	if e != nil {
		t.Fatal(e)
	}
	_, q, e = decodeAccount(raw)
	if e != nil {
		t.Fatal(e)
	}
	if q.items["a"].Questions[0].Answer != "secret" || q.items["a"].PlayerID != "p" {
		t.Fatal("lost private state")
	}
}

func exerciseDurableBackend(t *testing.T, b MySQLAccountBackend) {
	t.Helper()
	c, _, root := fixture(t)
	first := &wechatGateway{root: root, catalog: c, config: WeChatConfig{Backend: b}}
	second := &wechatGateway{root: root, catalog: c, config: WeChatConfig{Backend: b}}
	subject := strings.Repeat("a", 64)
	call := func(g *wechatGateway, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		g.durableAccount(w, httptest.NewRequest(method, path, strings.NewReader(body)), subject)
		return w
	}
	w := call(first, "POST", "/api/players", `{"name":"Amy","avatar":"fox"}`)
	var p Player
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(first, "POST", "/api/players/"+p.ID+"/attempts", `{"lessonId":1,"difficulty":"hard"}`)
	var a Attempt
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &a) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"Answer"`) {
		t.Fatal("answer leaked")
	}
	var answers []string
	if err := b.Update(context.Background(), subject, func(raw []byte) ([]byte, error) {
		_, q, err := decodeAccount(raw)
		if err != nil {
			return nil, err
		}
		for _, question := range q.items[a.ID].Questions {
			answers = append(answers, question.Answer)
		}
		return raw, nil
	}); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"answers": answers})
	path := "/api/players/" + p.ID + "/attempts/" + a.ID
	w = call(second, "POST", path, string(payload))
	var result Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Score != 5 || result.StarsEarned == 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	retry := call(first, "POST", path, string(payload))
	if retry.Body.String() != w.Body.String() {
		t.Fatal("retry changed result")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := call(second, "POST", "/api/players", `{"name":"Ben","avatar":"cat"}`); w.Code != 201 {
				t.Errorf("concurrent create: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	w = call(first, "GET", "/api/players", "")
	var players []Player
	if json.Unmarshal(w.Body.Bytes(), &players) != nil || len(players) != 13 {
		t.Fatalf("lost updates: %s", w.Body.String())
	}
}
func TestDurableCrossInstanceChallengesAndConcurrency(t *testing.T) {
	exerciseDurableBackend(t, &memoryAccounts{})
}

// Opt in only against a disposable existing MySQL database, never deployment credentials.
func TestMySQLActualIntegration(t *testing.T) {
	socket := os.Getenv("WECHAT_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("no disposable real MySQL configured; boundary tests are not real integration")
	}
	cfg := mysql.NewConfig()
	cfg.Net = "unix"
	cfg.Addr = socket
	cfg.User = "root"
	cfg.DBName = "english_adventure_test"
	cfg.Timeout = 2 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS english_wechat_accounts (account_id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,state MEDIUMBLOB NOT NULL) ENGINE=InnoDB`)
	if err != nil {
		t.Fatal(err)
	}
	b := &MySQLAccounts{db: db}
	subject := strings.Repeat("a", 64)
	if _, err = db.Exec("DELETE FROM english_wechat_accounts WHERE account_id=?", subject); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM english_wechat_accounts WHERE account_id=?", subject)
	exerciseDurableBackend(t, b)
	var before []byte
	if err = db.QueryRow("SELECT state FROM english_wechat_accounts WHERE account_id=?", subject).Scan(&before); err != nil {
		t.Fatal(err)
	}
	err = b.Update(context.Background(), subject, func([]byte) ([]byte, error) { return nil, errors.New("abort") })
	if err == nil {
		t.Fatal("callback failure accepted")
	}
	var after []byte
	if err = db.QueryRow("SELECT state FROM english_wechat_accounts WHERE account_id=?", subject).Scan(&after); err != nil || string(after) != string(before) {
		t.Fatal("rollback failed", err)
	}
}

func TestSQLTransactionFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"insert", "read", "callback", "write", "commit", "success"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			b := &MySQLAccounts{db: db}
			mock.ExpectBegin()
			insert := mock.ExpectExec(`INSERT INTO english_wechat_accounts (account_id,state) VALUES (?,?) ON DUPLICATE KEY UPDATE account_id=account_id`).WithArgs("account", []byte{})
			if stage == "insert" {
				insert.WillReturnError(errors.New("insert failed"))
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				read := mock.ExpectQuery(`SELECT state FROM english_wechat_accounts WHERE account_id=? FOR UPDATE`).WithArgs("account")
				if stage == "read" {
					read.WillReturnError(errors.New("read failed"))
				} else {
					read.WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow([]byte("old")))
					if stage != "callback" {
						write := mock.ExpectExec(`UPDATE english_wechat_accounts SET state=? WHERE account_id=?`).WithArgs([]byte("next"), "account")
						if stage == "write" {
							write.WillReturnError(errors.New("write failed"))
						} else {
							write.WillReturnResult(sqlmock.NewResult(0, 1))
							if stage == "commit" {
								mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
							} else {
								mock.ExpectCommit()
							}
						}
					}
				}
			}
			if stage != "success" && stage != "commit" {
				mock.ExpectRollback()
			}
			err = b.Update(context.Background(), "account", func(raw []byte) ([]byte, error) {
				if string(raw) != "old" {
					t.Fatalf("stale state %s", raw)
				}
				if stage == "callback" {
					return nil, errors.New("abort")
				}
				return []byte("next"), nil
			})
			if (err == nil) != (stage == "success") {
				t.Fatalf("stage %s err %v", stage, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadinessAndResponseBounds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	g := &wechatGateway{config: WeChatConfig{Backend: &MySQLAccounts{db: db}}}
	mock.ExpectPing().WillReturnError(errors.New("private password"))
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	buf := &boundedResponse{header: make(http.Header)}
	if _, err = buf.Write(make([]byte, maxResponseBytes+1)); err == nil || !buf.overflow {
		t.Fatal("unbounded response")
	}
	if _, _, err = decodeAccount(make([]byte, maxAccountBytes+1)); err == nil {
		t.Fatal("unbounded account")
	}
}
