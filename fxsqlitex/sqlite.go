package fxsqlitex

import (
	"context"
	"io/fs"
	"time"

	"github.com/exoscale/stelling/sqlite/migrationx"
	"go.uber.org/fx"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type DB struct {
	Path string `validate:"required"`
}

type DBConfig interface {
	DBConfig() *DB
}

func (db *DB) DBConfig() *DB {
	return db
}

// NewPool returns a new sqlitex.Pool of connections to the specified sqlite database
//
// It will ensure safe defaults are set, such as enabling the WAL and foreign keys
// Some parameters can be configured by passing in additional `Option`s
func NewPool(conf DBConfig, opts ...Option) (*sqlitex.Pool, error) {
	sqliteConf := evaluateOptions(conf, opts...)

	poolOpts := sqlitex.PoolOptions{
		PoolSize: int(sqliteConf.poolSize),
		Flags:    sqlite.OpenCreate | sqlite.OpenReadWrite | sqlite.OpenWAL,
	}

	if sqliteConf.inMemory {
		poolOpts.Flags |= sqlite.OpenMemory
		poolOpts.PoolSize = 1
	} else {
		poolOpts.Flags |= sqlite.OpenURI
	}

	poolOpts.PrepareConn = func(conn *sqlite.Conn) error {
		if sqliteConf.busyTimeout != time.Duration(0) {
			conn.SetBusyTimeout(sqliteConf.busyTimeout)
		}
		return sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", &sqlitex.ExecOptions{})
	}

	return sqlitex.NewPool(sqliteConf.path, poolOpts)
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
//
// This should almost NEVER be necessary: by default the pool will coordinate the lock via interrupts and
// Timeouts of the operation can be managed via the context
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

type ModuleOption func() fx.Option
type SchemaVersion uint64

func runMigrations(lc fx.Lifecycle, pool *sqlitex.Pool, migrations *migrationx.Migrations, version *SchemaVersion) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			conn, err := pool.Take(ctx)
			if err != nil {
				return err
			}
			defer pool.Put(conn)
			if version != nil {
				return migrations.Migrate(ctx, conn, uint64(*version))
			}
			return migrations.Up(ctx, conn)
		},
	})
}

// WithMigrations will run the migrations at the path in the given FS during system startup
// By default it will bring the schema to the latest version, but this can be tweaked by using
// the WithSchemaVersion ModuleOption
func WithMigrations(migrations fs.FS, path string) ModuleOption {
	return func() fx.Option {
		return fx.Module(
			"schema-migrator",
			fx.Supply(
				fx.Private,
				fx.Annotate(migrations, fx.As(new(fs.FS))),
				path,
			),
			fx.Provide(fx.Private, migrationx.NewMigrationsFromFS),
			fx.Invoke(
				fx.Annotate(runMigrations, fx.ParamTags("", "", "", `optional:"true"`)),
			),
		)
	}
}

// WithSchemaVersion will bring the schema to the given version rather than the latest version
// It has no effect if WithMigrations is not specified as well
func WithSchemaVersion(version uint64) ModuleOption {
	return func() fx.Option {
		schemaVersion := SchemaVersion(version)
		return fx.Supply(&schemaVersion)
	}
}

// WithSQLiteOptions will provide the given SqliteOptions to the sqlitex.Pool produced by the module
func WithSQLiteOptions(sqliteOpts ...Option) ModuleOption {
	return func() fx.Option {
		opts := make([]any, len(sqliteOpts))
		for i, opt := range sqliteOpts {
			opts[i] = fx.Annotate(opt, fx.ResultTags(`group:"sqlite_options"`))
		}
		return fx.Supply(opts...)
	}
}

// NewModule adds a module which provides an sqlitex.Pool to your system
func NewModule(conf DBConfig, opts ...ModuleOption) fx.Option {
	// Not wrapping this in fx.Module, so we can embed it more easily
	system := fx.Options(
		fx.Supply(
			fx.Private,
			fx.Annotate(conf, fx.As(new(DBConfig))),
		),
		fx.Provide(
			fx.Private,
			fx.Annotate(
				NewPool,
				fx.ParamTags("", `group:"sqlite_options"`),
				fx.OnStop(func(ctx context.Context, pool *sqlitex.Pool) error {
					return pool.Close()
				}),
			),
		),
	)
	for _, opt := range opts {
		system = fx.Options(system, opt())
	}

	return system
}
