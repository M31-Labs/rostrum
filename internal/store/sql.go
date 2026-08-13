package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/m31-labs/rostrum/internal/domain"
	_ "modernc.org/sqlite"
)

const (
	workspaceTable       = "rostrum_workspace"
	migrationsTable      = "rostrum_schema_migrations"
	workspaceSchemaLevel = 1
)

// SQLStore stores the entire validated workspace document as one versioned
// JSON row. That deliberately mirrors JSONStore's transactional semantics:
// application code continues to work with one State aggregate while SQLite
// and Postgres provide durable coordination, backups, and operational
// tooling. A later relational projection can be added without splitting the
// canonical consistency boundary prematurely.
type SQLStore struct {
	mu       sync.RWMutex
	db       *sql.DB
	dialect  string
	path     string
	seed     domain.State
	state    domain.State
	snapshot domain.State
	version  int64
}

// OpenSQLite opens (or creates) a SQLite-backed workspace. path may be a
// normal filesystem path or :memory: for an isolated in-memory workspace.
func OpenSQLite(path string, seed domain.State) (*SQLStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite store requires a data path")
	}
	if path != ":memory:" {
		path = filepath.Clean(path)
		if err := prepareSQLitePath(path); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}
	// StateStore already serializes local mutations. One connection also keeps
	// :memory: SQLite databases stable (each SQLite in-memory connection is a
	// different database) and avoids needless lock contention for file-backed
	// workspaces.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if path != ":memory:" {
		var journalMode string
		if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable sqlite WAL: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(journalMode), "wal") {
			_ = db.Close()
			return nil, fmt.Errorf("enable sqlite WAL: database returned journal mode %q", journalMode)
		}
		if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite WAL durability: %w", err)
		}
	}
	// A short busy timeout turns ordinary concurrent HTTP writes into normal
	// serialized transactions rather than a surprising immediate SQLITE_BUSY.
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite busy timeout: %w", err)
	}
	store, err := openSQLStore(db, "sqlite", "sqlite:"+path, seed)
	if err != nil {
		return nil, err
	}
	if path != ":memory:" {
		if err := secureSQLiteArtifacts(path); err != nil {
			_ = store.Close()
			return nil, err
		}
	}
	return store, nil
}

func prepareSQLitePath(path string) error {
	directory := filepath.Dir(path)
	_, statErr := os.Stat(directory)
	createdDirectory := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !createdDirectory {
		return fmt.Errorf("inspect sqlite data directory: %w", statErr)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create sqlite data directory: %w", err)
	}
	if createdDirectory {
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("secure new sqlite data directory: %w", err)
		}
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("sqlite data path must be a regular non-symlink file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect sqlite data path: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("prepare sqlite data file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close sqlite data file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure sqlite data file: %w", err)
	}
	return nil
}

func secureSQLiteArtifacts(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("secure sqlite artifact %s: %w", filepath.Base(candidate), err)
		}
	}
	return nil
}

// OpenPostgres opens a Postgres-backed workspace. Credentials stay solely in
// the connection URL/environment and Path intentionally never echoes them.
func OpenPostgres(databaseURL string, seed domain.State) (*SQLStore, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("postgres store requires DATABASE_URL")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres store: %w", err)
	}
	return openSQLStore(db, "postgres", postgresPath(databaseURL), seed)
}

// OpenConfigured selects the configured backend while retaining JSON as the
// zero-config reference implementation. STORE_DRIVER accepts json, sqlite,
// postgres, or postgresql.
func OpenConfigured(driver, dataPath, databaseURL string, seed domain.State) (StateStore, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "json":
		return Open(dataPath, seed)
	case "sqlite":
		return OpenSQLite(dataPath, seed)
	case "postgres", "postgresql":
		return OpenPostgres(databaseURL, seed)
	default:
		return nil, fmt.Errorf("unsupported STORE_DRIVER %q (use json, sqlite, or postgres)", driver)
	}
}

func openSQLStore(db *sql.DB, dialect, path string, seed domain.State) (*SQLStore, error) {
	if err := seed.Validate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("validate seed state: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect %s store: %w", dialect, err)
	}
	store := &SQLStore{db: db, dialect: dialect, path: path, seed: clone(seed)}
	if err := store.applyMigrations(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.loadOrSeed(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// applyMigrations keeps the aggregate table intentionally compact while
// making SQL startup explicit and forward-compatible. A pre-migration v0
// database is adopted by creating the migration ledger and recording level 1;
// future migrations append another numbered entry rather than guessing from
// the table shape.
func (store *SQLStore) applyMigrations() error {
	// TEXT works for JSON and timestamp values in both SQLite and Postgres;
	// version is used to invalidate a process-local immutable snapshot when a
	// second process commits a workspace change.
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS ` + workspaceTable + ` (
		id INTEGER PRIMARY KEY,
		version BIGINT NOT NULL,
		state TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create workspace table: %w", err)
	}
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS ` + migrationsTable + ` (
		version BIGINT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create store migrations table: %w", err)
	}
	var recorded bool
	err := store.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM ` + migrationsTable + ` WHERE version = ` + fmt.Sprint(workspaceSchemaLevel) + `)`).Scan(&recorded)
	if err != nil {
		return fmt.Errorf("read store migration level: %w", err)
	}
	if !recorded {
		if _, err := store.db.Exec(store.insertMigrationSQL(), workspaceSchemaLevel, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record store migration %d: %w", workspaceSchemaLevel, err)
		}
	}
	return nil
}

func (store *SQLStore) loadOrSeed(ctx context.Context) error {
	var raw string
	var version int64
	err := store.db.QueryRowContext(ctx, `SELECT version, state FROM `+workspaceTable+` WHERE id = 1`).Scan(&version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		encoded, marshalErr := encodeState(store.seed)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err := store.db.ExecContext(ctx, store.insertSQL(), int64(1), encoded, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("seed %s store: %w", store.dialect, err)
		}
		store.state = clone(store.seed)
		store.snapshot = clone(store.seed)
		store.version = 1
		return nil
	}
	if err != nil {
		return fmt.Errorf("load %s store: %w", store.dialect, err)
	}
	state, err := decodeState(raw)
	if err != nil {
		return fmt.Errorf("decode %s store: %w", store.dialect, err)
	}
	store.state = state
	store.snapshot = clone(state)
	store.version = version
	return nil
}

// Snapshot returns the latest immutable view. Unlike JSONStore, this checks
// the small row version first so a second application process can publish an
// update without requiring a restart to make this process observe it.
func (store *SQLStore) Snapshot() domain.State {
	store.mu.Lock()
	defer store.mu.Unlock()
	var version int64
	if err := store.db.QueryRow(`SELECT version FROM ` + workspaceTable + ` WHERE id = 1`).Scan(&version); err == nil && version != store.version {
		if err := store.loadOrSeed(context.Background()); err != nil {
			panic(fmt.Sprintf("refresh %s store snapshot: %v", store.dialect, err))
		}
	}
	return store.snapshot
}

func (store *SQLStore) Update(change func(*domain.State) error) error {
	return store.UpdateAudit(GenericAudit, change)
}

func (store *SQLStore) UpdateAudit(meta domain.AuditMeta, change func(*domain.State) error) error {
	if change == nil {
		return errors.New("store update requires a mutation")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	transaction, err := store.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin %s update: %w", store.dialect, err)
	}
	defer func() { _ = transaction.Rollback() }()

	state, version, err := store.readLocked(transaction)
	if err != nil {
		return err
	}
	next := clone(state)
	if err := change(&next); err != nil {
		return err
	}
	next.AppendAudit(meta)
	if err := store.persistTransaction(transaction, next, version+1); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit %s update: %w", store.dialect, err)
	}
	store.state = next
	store.snapshot = clone(next)
	store.version = version + 1
	return nil
}

func (store *SQLStore) Reset() error {
	return store.Replace(clone(store.seed), domain.AuditMeta{
		Actor:      "system",
		Action:     "workspace.reset",
		EntityType: "workspace",
		Summary:    "Workspace reset to its configured initial state.",
		Origin:     "rostrum",
	})
}

func (store *SQLStore) Replace(next domain.State, meta domain.AuditMeta) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	transaction, err := store.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin %s replacement: %w", store.dialect, err)
	}
	defer func() { _ = transaction.Rollback() }()
	_, version, err := store.readLocked(transaction)
	if err != nil {
		return err
	}
	next = clone(next)
	next.SchemaVersion = domain.CurrentSchemaVersion
	next.UpdatedAt = time.Now().UTC()
	next.AppendAudit(meta)
	if err := store.persistTransaction(transaction, next, version+1); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit %s replacement: %w", store.dialect, err)
	}
	store.state = next
	store.snapshot = clone(next)
	store.version = version + 1
	return nil
}

func (store *SQLStore) readLocked(transaction *sql.Tx) (domain.State, int64, error) {
	query := `SELECT version, state FROM ` + workspaceTable + ` WHERE id = 1`
	if store.dialect == "postgres" {
		query += " FOR UPDATE"
	}
	var raw string
	var version int64
	if err := transaction.QueryRow(query).Scan(&version, &raw); err != nil {
		return domain.State{}, 0, fmt.Errorf("read %s workspace: %w", store.dialect, err)
	}
	state, err := decodeState(raw)
	if err != nil {
		return domain.State{}, 0, fmt.Errorf("decode %s workspace: %w", store.dialect, err)
	}
	return state, version, nil
}

func (store *SQLStore) persistTransaction(transaction *sql.Tx, next domain.State, version int64) error {
	next.SchemaVersion = domain.CurrentSchemaVersion
	next.UpdatedAt = time.Now().UTC()
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate update: %w", err)
	}
	encoded, err := encodeState(next)
	if err != nil {
		return err
	}
	if _, err := transaction.Exec(store.updateSQL(), version, encoded, next.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist %s workspace: %w", store.dialect, err)
	}
	return nil
}

func (store *SQLStore) insertSQL() string {
	if store.dialect == "postgres" {
		return `INSERT INTO ` + workspaceTable + ` (id, version, state, updated_at) VALUES (1, $1, $2, $3)`
	}
	return `INSERT INTO ` + workspaceTable + ` (id, version, state, updated_at) VALUES (1, ?, ?, ?)`
}

func (store *SQLStore) updateSQL() string {
	if store.dialect == "postgres" {
		return `UPDATE ` + workspaceTable + ` SET version = $1, state = $2, updated_at = $3 WHERE id = 1`
	}
	return `UPDATE ` + workspaceTable + ` SET version = ?, state = ?, updated_at = ? WHERE id = 1`
}

func (store *SQLStore) insertMigrationSQL() string {
	if store.dialect == "postgres" {
		return `INSERT INTO ` + migrationsTable + ` (version, applied_at) VALUES ($1, $2)`
	}
	return `INSERT INTO ` + migrationsTable + ` (version, applied_at) VALUES (?, ?)`
}

func encodeState(state domain.State) (string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode state: %w", err)
	}
	return string(data), nil
}

func decodeState(raw string) (domain.State, error) {
	var state domain.State
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return domain.State{}, err
	}
	if state.SchemaVersion != domain.CurrentSchemaVersion {
		return domain.State{}, fmt.Errorf("data schema %d is not supported; want %d", state.SchemaVersion, domain.CurrentSchemaVersion)
	}
	if err := state.Validate(); err != nil {
		return domain.State{}, err
	}
	return state, nil
}

func (store *SQLStore) Path() string { return store.path }

func (store *SQLStore) Close() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.db == nil {
		return nil
	}
	err := store.db.Close()
	store.db = nil
	return err
}

func postgresPath(databaseURL string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "postgres"
	}
	path := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if path == "" {
		path = "default"
	}
	if host := parsed.Hostname(); host != "" {
		return "postgres://" + host + "/" + path
	}
	return "postgres/" + path
}
