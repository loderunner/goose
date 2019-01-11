package goose

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MigrationRecord struct.
type MigrationRecord struct {
	VersionID int64
	TStamp    time.Time
	IsApplied bool // was this a result of up() or down()
}

// QueryExecer is an umbrella interface that regroups the Query and Exec
// functions of both sql.DB and sql.Tx
type QueryExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// Migration struct.
type Migration struct {
	Version    int64
	Next       int64  // next version, or -1 if none
	Previous   int64  // previous version, -1 if none
	Source     string // path to .sql script
	Registered bool
	UpFn       func(QueryExecer) error // Up go migration function
	DownFn     func(QueryExecer) error // Down go migration function
	NoTx       bool
}

func (m *Migration) String() string {
	return fmt.Sprintf(m.Source)
}

// Up runs an up migration.
func (m *Migration) Up(db *sql.DB) error {
	if err := m.run(db, true); err != nil {
		return err
	}
	log.Println("OK   ", filepath.Base(m.Source))
	return nil
}

// Down runs a down migration.
func (m *Migration) Down(db *sql.DB) error {
	if err := m.run(db, false); err != nil {
		return err
	}
	log.Println("OK   ", filepath.Base(m.Source))
	return nil
}

func (m *Migration) run(db *sql.DB, direction bool) error {
	switch filepath.Ext(m.Source) {
	case ".sql":
		if err := runSQLMigration(db, m.Source, m.Version, direction); err != nil {
			return fmt.Errorf("FAIL %v, quitting migration", err)
		}

	case ".go":
		if !m.Registered {
			log.Fatalf("failed to apply Go migration %q: Go functions must be registered and built into a custom binary (see https://github.com/loderunner/goose/tree/master/examples/go-migrations)", m.Source)
		}

		if m.NoTx {
			fn := m.UpFn
			if !direction {
				fn = m.DownFn
			}
			if fn != nil {
				if err := fn(db); err != nil {
					log.Fatalf("FAIL %s (%v), quitting migration.", filepath.Base(m.Source), err)
					return err
				}
			}
			if direction {
				if _, err := db.Exec(GetDialect().insertVersionSQL(), m.Version, direction); err != nil {
					return err
				}
			} else {
				if _, err := db.Exec(GetDialect().deleteVersionSQL(), m.Version); err != nil {
					return err
				}
			}
		} else {
			tx, err := db.Begin()
			if err != nil {
				log.Fatal("db.Begin: ", err)
			}

			fn := m.UpFn
			if !direction {
				fn = m.DownFn
			}
			if fn != nil {
				if err := fn(tx); err != nil {
					tx.Rollback()
					log.Fatalf("FAIL %s (%v), quitting migration.", filepath.Base(m.Source), err)
					return err
				}
			}
			if direction {
				if _, err := tx.Exec(GetDialect().insertVersionSQL(), m.Version, direction); err != nil {
					tx.Rollback()
					return err
				}
			} else {
				if _, err := tx.Exec(GetDialect().deleteVersionSQL(), m.Version); err != nil {
					tx.Rollback()
					return err
				}
			}

			return tx.Commit()
		}
	}

	return nil
}

// NumericComponent looks for migration scripts with names in the form:
// XXX_descriptivename.ext where XXX specifies the version number
// and ext specifies the type of migration
func NumericComponent(name string) (int64, error) {

	base := filepath.Base(name)

	if ext := filepath.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("not a recognized migration file type")
	}

	idx := strings.Index(base, "_")
	if idx < 0 {
		return 0, errors.New("no separator found")
	}

	n, e := strconv.ParseInt(base[:idx], 10, 64)
	if e == nil && n <= 0 {
		return 0, errors.New("migration IDs must be greater than zero")
	}

	return n, e
}
