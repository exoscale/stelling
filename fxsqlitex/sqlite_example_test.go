package fxsqlitex_test

import (
	"context"
	"fmt"
	"testing/fstest"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxsqlitex"
	"go.uber.org/fx"
	gsqlite "zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type Config struct {
	fxsqlitex.DB
}

func Example() {
	conf := &Config{}
	args := []string{"sqlite-test", "--db.path", "/tmp/db.sqlite"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	migrations := makeTestFs()

	app := fx.New(fx.Options(
		fxsqlitex.NewModule(
			conf,
			fxsqlitex.WithMigrations(migrations, "."),
			fxsqlitex.WithSQLiteOptions(fxsqlitex.WithInMemory()),
		),
		fx.Invoke(run),
	))

	app.Run()

	// Output:
	// name test
	// value 1
}

func run(lc fx.Lifecycle, sd fx.Shutdowner, pool *sqlitex.Pool) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			conn, err := pool.Take(ctx)
			if err != nil {
				return err
			}
			defer pool.Put(conn)

			name := ""
			value := 0
			if err := sqlitex.ExecuteTransient(
				conn,
				"SELECT name, value FROM example;",
				&sqlitex.ExecOptions{
					ResultFunc: func(stmt *gsqlite.Stmt) error {
						name = stmt.ColumnText(0)
						value = stmt.ColumnInt(1)
						return nil
					},
				},
			); err != nil {
				return err
			}
			fmt.Println("name", name)
			fmt.Println("value", value)
			return sd.Shutdown()
		},
	})
}

const schemaUp = `CREATE TABLE IF NOT EXISTS example (
	name text,
	value number,
	PRIMARY KEY(name)
);
INSERT INTO example(name, value) VALUES ('test', 1);`
const schemaDown = `DELETE TABLE IF EXISTS exampe;`

func makeTestFs() fstest.MapFS {
	output := make(map[string]*fstest.MapFile)

	output["0001_init_schema.up.sql"] = &fstest.MapFile{
		Data: []byte(schemaUp),
	}
	output["0001_init_schema.down.sql"] = &fstest.MapFile{
		Data: []byte(schemaDown),
	}
	return output
}
