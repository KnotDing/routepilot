package database

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"surge-controller/internal/config"
	"time"
)

type Store struct {
	DB  *sql.DB
	key []byte
}
type Event struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	Severity string    `json:"severity"`
	Kind     string    `json:"kind"`
	Message  string    `json:"message"`
}

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("database path must be absolute")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	if info, e := os.Stat(filepath.Dir(path)); e != nil || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("database directory must be private (0700)")
	}
	for _, p := range []string{path, path + ".key"} {
		if info, e := os.Lstat(p); e == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return nil, errors.New("database/key cannot be a link or special file")
		}
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return nil, e
	}
	f.Close()
	key, e := os.ReadFile(path + ".key")
	if os.IsNotExist(e) {
		key = make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			return nil, e
		}
		f, e := os.OpenFile(path+".key", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return nil, e
		}
		_, e = f.Write(key)
		ce := f.Close()
		if e != nil {
			return nil, e
		}
		if ce != nil {
			return nil, ce
		}
	} else if e != nil {
		return nil, e
	}
	ki, e := os.Stat(path + ".key")
	if e != nil || len(key) != 32 || ki.Mode().Perm()&0077 != 0 {
		return nil, errors.New("invalid or non-private encryption key")
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db, key}
	if e = s.migrate(); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) migrate() error {
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL"} {
		if _, e := s.DB.Exec(q); e != nil {
			return e
		}
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)"); e != nil {
		return e
	}
	var max sql.NullInt64
	if e = tx.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&max); e != nil {
		return e
	}
	if max.Valid && max.Int64 > 4 {
		return errors.New("database schema is newer than this application")
	}
	for _, table := range []string{"settings", "bgp_peers", "bgp_routes", "bgp_route_feeds", "dns_interception_routes", "overrides", "local_dns_hosts"} {
		if _, e = tx.Exec("CREATE TABLE IF NOT EXISTS " + table + "(id TEXT PRIMARY KEY, document BLOB NOT NULL)"); e != nil {
			return e
		}
	}
	if _, e = tx.Exec("CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, severity TEXT NOT NULL, kind TEXT NOT NULL, message TEXT NOT NULL)"); e != nil {
		return e
	}
	if _, e = tx.Exec("CREATE TABLE IF NOT EXISTS statistics(id TEXT PRIMARY KEY, document BLOB NOT NULL)"); e != nil {
		return e
	}
	if _, e = tx.Exec("CREATE TABLE IF NOT EXISTS decision_cache(id TEXT PRIMARY KEY, document BLOB NOT NULL)"); e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT OR IGNORE INTO schema_migrations VALUES(1,?)", time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT OR IGNORE INTO schema_migrations VALUES(2,?)", time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT OR IGNORE INTO schema_migrations VALUES(3,?)", time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT OR IGNORE INTO schema_migrations VALUES(4,?)", time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) LoadDecisionCache(ctx context.Context, value any) error {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT document FROM decision_cache WHERE id='current'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}

func (s *Store) SaveDecisionCache(ctx context.Context, raw []byte) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO decision_cache(id,document) VALUES('current',?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", raw)
	return err
}

func (s *Store) LoadStatistics(ctx context.Context, id string, value any) error {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT document FROM statistics WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}

func (s *Store) SaveStatistics(ctx context.Context, id string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO statistics(id,document) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", id, raw)
	return err
}
func (s *Store) crypt(value string, encrypt bool) (string, error) {
	block, e := aes.NewCipher(s.key)
	if e != nil {
		return "", e
	}
	a, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	if encrypt {
		nonce := make([]byte, a.NonceSize())
		if _, e = rand.Read(nonce); e != nil {
			return "", e
		}
		return base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(value), []byte("surge-api-token-v1"))), nil
	}
	raw, e := base64.StdEncoding.DecodeString(value)
	if e != nil || len(raw) < a.NonceSize() {
		return "", errors.New("invalid stored secret")
	}
	plain, e := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte("surge-api-token-v1"))
	return string(plain), e
}
func (s *Store) Save(ctx context.Context, d config.Document) error {
	if e := d.Validate(); e != nil {
		return e
	}
	if d.Settings.Notifications.WebhookURL != "" {
		v, err := s.crypt(d.Settings.Notifications.WebhookURL, true)
		if err != nil {
			return err
		}
		d.Settings.Notifications.WebhookURL = v
	}
	if d.Settings.Surge.APIToken != "" {
		v, e := s.crypt(d.Settings.Surge.APIToken, true)
		if e != nil {
			return e
		}
		d.Settings.Surge.APIToken = v
	}
	d.Settings.Security.Password = ""
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	b, e := json.Marshal(d.Settings)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO settings VALUES('main',?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", b); e != nil {
		return e
	}
	lists := []struct {
		table string
		items any
	}{{"bgp_peers", d.Peers}, {"bgp_routes", d.Routes}, {"bgp_route_feeds", d.RouteFeeds}, {"dns_interception_routes", d.Interception}, {"overrides", d.Overrides}, {"local_dns_hosts", d.Hosts}}
	for _, list := range lists {
		raw, _ := json.Marshal(list.items)
		var rows []json.RawMessage
		if e = json.Unmarshal(raw, &rows); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM "+list.table); e != nil {
			return e
		}
		for _, r := range rows {
			var id struct {
				ID string `json:"id"`
			}
			if e = json.Unmarshal(r, &id); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO "+list.table+" VALUES(?,?)", id.ID, []byte(r)); e != nil {
				return e
			}
		}
	}
	return tx.Commit()
}
func (s *Store) Load(ctx context.Context) (config.Document, error) {
	d := config.Default()
	var b []byte
	e := s.DB.QueryRowContext(ctx, "SELECT document FROM settings WHERE id='main'").Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return d, nil
	}
	if e != nil {
		return d, e
	}
	if e = json.Unmarshal(b, &d.Settings); e != nil {
		return d, e
	}
	if d.Settings.Notifications.WebhookURL != "" {
		d.Settings.Notifications.WebhookURL, e = s.crypt(d.Settings.Notifications.WebhookURL, false)
		if e != nil {
			return d, e
		}
	}
	if d.Settings.Surge.APIToken != "" {
		d.Settings.Surge.APIToken, e = s.crypt(d.Settings.Surge.APIToken, false)
		if e != nil {
			return d, e
		}
	}
	for _, dest := range []struct {
		table string
		out   any
	}{{"bgp_peers", &d.Peers}, {"bgp_routes", &d.Routes}, {"bgp_route_feeds", &d.RouteFeeds}, {"dns_interception_routes", &d.Interception}, {"overrides", &d.Overrides}, {"local_dns_hosts", &d.Hosts}} {
		rows, e := s.DB.QueryContext(ctx, "SELECT document FROM "+dest.table+" ORDER BY id")
		if e != nil {
			return d, e
		}
		items := []json.RawMessage{}
		for rows.Next() {
			var b []byte
			if e = rows.Scan(&b); e != nil {
				rows.Close()
				return d, e
			}
			items = append(items, json.RawMessage(b))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return d, e
		}
		b, _ := json.Marshal(items)
		if e = json.Unmarshal(b, dest.out); e != nil {
			return d, e
		}
	}
	return d, d.Validate()
}
func (s *Store) Event(ctx context.Context, severity, kind, message string, retention int) (Event, error) {
	ev := Event{At: time.Now().UTC(), Severity: severity, Kind: kind, Message: message}
	r, e := s.DB.ExecContext(ctx, "INSERT INTO events(at,severity,kind,message) VALUES(?,?,?,?)", ev.At.Format(time.RFC3339Nano), severity, kind, message)
	if e != nil {
		return ev, e
	}
	ev.ID, e = r.LastInsertId()
	if e != nil {
		return ev, e
	}
	_, e = s.DB.ExecContext(ctx, "DELETE FROM events WHERE id <= (SELECT COALESCE(MAX(id),0)-? FROM events)", retention)
	return ev, e
}
func (s *Store) Events(ctx context.Context, after int64) ([]Event, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,at,severity,kind,message FROM events WHERE id>? ORDER BY id DESC LIMIT 500", after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var ev Event
		var at string
		if e = rows.Scan(&ev.ID, &at, &ev.Severity, &ev.Kind, &ev.Message); e != nil {
			return nil, e
		}
		ev.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, ev)
	}
	return out, rows.Err()
}
func (s *Store) Close() error {
	_, e := s.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	ce := s.DB.Close()
	if e != nil {
		return e
	}
	return ce
}
