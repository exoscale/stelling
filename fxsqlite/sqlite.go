package fxsqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/exoscale/stelling/fxsqlitex"
	_ "github.com/mattn/go-sqlite3"
)

type DB fxsqlitex.DB

func (db *DB) DBConfig() *DB {
	return db
}

type DBConfig interface {
	DBConfig() *DB
}

func NewDB(conf DBConfig, opts ...Option) (*sql.DB, error) {
	sqliteConf := evaluateOptions(conf, opts...)

	dsn := fmt.Sprintf("file:%s?_journal_mode=wal&_fk=true", sqliteConf.path)
	if sqliteConf.busyTimeout != 0 {
		dsn = fmt.Sprintf("%s&_busy_timeout=%d", dsn, sqliteConf.busyTimeout.Milliseconds())
	}
	// in-memory database overwrite the full string
	if sqliteConf.inMemory {
		dsn = ":memory:?cache=shared"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db %v: %w", dsn, err)
	}

	// If you try to open a sqlite3 db that doesn't exist and on a place where a file can't be
	// created, sql.Open will succeed but any operation will fail. This ensure that the failure is
	// visible at the right place
	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("open db %v: %w", dsn, err)
	}

	db.SetMaxOpenConns(int(sqliteConf.poolSize))

	if sqliteConf.inMemory {
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(-1)
		db.SetMaxOpenConns(1)
	}

	return db, nil
}

type sqliteConfig struct {
	path        string
	poolSize    uint
	inMemory    bool
	busyTimeout time.Duration
}

type Option func(*sqliteConfig)

// WithInMemory will create a new in memory database rather than opening an existing one from disk
func WithInMemory() Option {
	return func(c *sqliteConfig) { c.inMemory = true }
}

// WithPoolSize sets the maximum number of connections to the sqlite database
// The default value is 1, since sqlite only allows a single thread to write, but for read intensive
// applications it can be useful to increase this value
func WithPoolSize(poolSize uint) Option {
	return func(c *sqliteConfig) { c.poolSize = poolSize }
}

// WithBusyTimeout sets a busy handler on each connection that waits up to Duration to acquire the write lock
// See: https://www.sqlite.org/c3ref/busy_timeout.html
func WithBusyTimeout(busyTimeout time.Duration) Option {
	return func(c *sqliteConfig) { c.busyTimeout = busyTimeout }
}

func evaluateOptions(conf DBConfig, opts ...Option) *sqliteConfig {
	path := ""
	if conf != nil {
		path = conf.DBConfig().Path
	}
	sqliteConf := &sqliteConfig{
		path:     path,
		poolSize: 1,
		inMemory: false,
	}
	for _, opt := range opts {
		opt(sqliteConf)
	}
	return sqliteConf
}
