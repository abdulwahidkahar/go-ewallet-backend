//go:build integration

package it

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

type integrationDB struct {
	adminDB    *sql.DB
	db         *sql.DB
	schemaName string
}

func newIntegrationDB(t *testing.T) *integrationDB {
	t.Helper()

	baseDSN := integrationBaseDSN()

	adminDB, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("failed to open admin db: %v", err)
	}

	if err := adminDB.Ping(); err != nil {
		adminDB.Close()
		t.Fatalf("failed to ping postgres, run make docker.start.components first: %v", err)
	}

	schemaName := fmt.Sprintf("ewallet_it_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec(`CREATE SCHEMA ` + schemaName); err != nil {
		adminDB.Close()
		t.Fatalf("failed to create schema: %v", err)
	}

	testDSN, err := withSearchPath(baseDSN, schemaName)
	if err != nil {
		adminDB.Close()
		t.Fatalf("failed to build dsn with search_path: %v", err)
	}

	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)
		adminDB.Close()
		t.Fatalf("failed to open test db: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		db.Close()
		adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)
		adminDB.Close()
		t.Fatalf("failed to ping test db: %v", err)
	}

	applyMigrations(t, db)

	store := &integrationDB{
		adminDB:    adminDB,
		db:         db,
		schemaName: schemaName,
	}

	t.Cleanup(func() {
		store.close(t)
	})

	return store
}

func (i *integrationDB) close(t *testing.T) {
	t.Helper()

	if i.db != nil {
		if err := i.db.Close(); err != nil {
			t.Errorf("failed to close test db: %v", err)
		}
	}

	if i.adminDB != nil {
		if _, err := i.adminDB.Exec(`DROP SCHEMA IF EXISTS ` + i.schemaName + ` CASCADE`); err != nil {
			t.Errorf("failed to drop schema %s: %v", i.schemaName, err)
		}
		if err := i.adminDB.Close(); err != nil {
			t.Errorf("failed to close admin db: %v", err)
		}
	}
}

func integrationBaseDSN() string {
	if dsn := os.Getenv("EWALLET_TEST_DSN"); dsn != "" {
		return dsn
	}

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	password := getenv("DB_PASSWORD", "password")
	name := getenv("DB_NAME", "auth_api")

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user,
		password,
		host,
		port,
		name,
	)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func withSearchPath(baseDSN, schema string) (string, error) {
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, path := range migrationFiles(t) {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read migration %s: %v", path, err)
		}

		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("failed to execute migration %s: %v", path, err)
		}
	}
}

func migrationFiles(t *testing.T) []string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve integration test path")
	}

	pattern := filepath.Join(filepath.Dir(currentFile), "..", "migrations", "*.up.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to list migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	sort.Strings(files)
	return files
}
