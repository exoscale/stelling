package fxsqlite

import (
	"context"
	"database/sql"
	"io/fs"

	"github.com/exoscale/stelling/sqlite/migration"
	"go.uber.org/fx"
)

type ModuleOption func() fx.Option
type SchemaVersion uint64

func runMigrations(lc fx.Lifecycle, db *sql.DB, migrations *migration.Migrations, version *SchemaVersion) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if version != nil {
				return migrations.Migrate(ctx, db, uint64(*version))
			}
			return migrations.Up(ctx, db)
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
			fx.Provide(fx.Private, migration.NewMigrationsFromFS),
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

// WithSQLiteOptions will provide the given SqliteOptions to the sql.DB produced by the module
func WithSQLiteOptions(sqliteOpts ...Option) ModuleOption {
	return func() fx.Option {
		opts := make([]any, len(sqliteOpts))
		for i, opt := range sqliteOpts {
			opts[i] = fx.Annotate(opt, fx.ResultTags(`group:"sqlite_options"`))
		}
		return fx.Supply(opts...)
	}
}

// NewModule adds a module which provides an *sql.DB to your system
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
				NewDB,
				fx.ParamTags("", `group:"sqlite_options"`),
				fx.OnStop(func(ctx context.Context, db *sql.DB) error {
					return db.Close()
				}),
			),
			fx.Annotate(
				NewGormDB,
				fx.ParamTags("", `optional:"true"`),
			),
		),
	)
	for _, opt := range opts {
		system = fx.Options(system, opt())
	}

	return system
}
