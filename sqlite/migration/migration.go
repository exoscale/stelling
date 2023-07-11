package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/exoscale/stelling/sqlite/migrationx"
)

// sqlExecutor is a simple interface which unifies Tx, DB and Conn
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ensureVersionSchema(ctx context.Context, tx sqlExecutor) error {
	_, err := tx.ExecContext(ctx, migrationx.VersionSchema)
	return err
}

func dbVersion(ctx context.Context, tx sqlExecutor) (uint64, error) {
	var version uint64
	row := tx.QueryRowContext(
		ctx,
		"SELECT version FROM schema_migrations LIMIT 1",
	)
	err := row.Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return version, err
}

func setDbVersion(ctx context.Context, tx sqlExecutor, version uint64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations"); err != nil {
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)",
		version,
		false,
	)
	return err
}

type Migrations struct {
	*migrationx.Migrations
}

func NewMigrations(up []string, down []string) (*Migrations, error) {
	m, err := migrationx.NewMigrations(up, down)
	if err != nil {
		return nil, err
	}
	return &Migrations{Migrations: m}, nil
}

func NewMigrationsFromFS(fsys fs.FS, subpath string) (*Migrations, error) {
	m, err := migrationx.NewMigrationsFromFS(fsys, subpath)
	if err != nil {
		return nil, err
	}
	return &Migrations{Migrations: m}, nil
}

func (m *Migrations) Up(ctx context.Context, db *sql.DB) error {
	targetVersion := uint64(len(m.UpScripts))
	return m.Migrate(ctx, db, targetVersion)
}

func (m *Migrations) Down(ctx context.Context, db *sql.DB) error {
	return m.Migrate(ctx, db, 0)
}

func (m *Migrations) Migrate(ctx context.Context, db *sql.DB, targetVersion uint64) error {
	if uint64(len(m.UpScripts)) < targetVersion {
		return fmt.Errorf("migrate failed: target version %d is higher than max migration version %d", targetVersion, len(m.UpScripts))
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}

	if err := ensureVersionSchema(ctx, tx); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
		}
		return fmt.Errorf("migrate failed: %w", err)
	}

	version, err := dbVersion(ctx, tx)
	if err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
		}
		return fmt.Errorf("migrate failed: %w", err)
	}

	if version == targetVersion {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate failed: %w", err)
		}
		return nil
	}

	if uint64(len(m.UpScripts)) < version {
		err := fmt.Errorf("database version %d is higher than max migration version %d", version, len(m.UpScripts))
		if err2 := tx.Rollback(); err2 != nil {
			return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
		}
		return fmt.Errorf("migrate failed: %w", err)
	}

	if targetVersion < version {
		for i := int(version - 1); i >= int(targetVersion); i-- {
			_, err := tx.ExecContext(ctx, m.DownScripts[i])
			if err != nil {
				if err2 := tx.Rollback(); err2 != nil {
					return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
				}
				return fmt.Errorf("migrate failed: %w", err)
			}
		}
	} else {
		for _, migration := range m.UpScripts[version:targetVersion] {
			_, err := tx.ExecContext(ctx, migration)
			if err != nil {
				if err2 := tx.Rollback(); err2 != nil {
					return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
				}
				return fmt.Errorf("migrate failed: %w", err)
			}
		}
	}

	if err := setDbVersion(ctx, tx, targetVersion); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return fmt.Errorf("migrate failed: %w, rollback failed: %w", err, err2)
		}
		return fmt.Errorf("migrate failed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}
	return nil
}
