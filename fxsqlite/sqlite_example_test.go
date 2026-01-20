package fxsqlite_test

import (
	"context"
	"fmt"
	"testing/fstest"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxsqlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Config struct {
	fxsqlite.DB
}

func Example() {
	conf := &Config{}
	args := []string{"sqlite-test", "--db.path", "/tmp/db.sqlite"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	migrations := makeTestFs()

	app := fx.New(fx.Options(
		fxsqlite.NewModule(
			conf,
			fxsqlite.WithMigrations(migrations, "."),
			fxsqlite.WithSQLiteOptions(fxsqlite.WithInMemory()),
		),
		fx.Invoke(run),
	))

	app.Run()

	// Output:
	// name test
	// value 1
}

type example struct {
	Name  string `gorm:"name"`
	Value int    `gorm:"value"`
}

func run(lc fx.Lifecycle, sd fx.Shutdowner, db *gorm.DB) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			row, err := gorm.G[example](db).First(ctx)
			if err != nil {
				return err
			}
			fmt.Println("name", row.Name)
			fmt.Println("value", row.Value)
			return sd.Shutdown()
		},
	})
}

const schemaUp = `CREATE TABLE IF NOT EXISTS examples (
	name text,
	value number,
	PRIMARY KEY(name)
);
INSERT INTO examples(name, value) VALUES ('test', 1);`
const schemaDown = `DELETE TABLE IF EXISTS exampes;`

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
