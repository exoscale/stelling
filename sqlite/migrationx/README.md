# Sqlite Migrationx

Provides a small utility package which performs SQLite database migrations in a single atomic transaction.

The API is modeled on [golang-migrate](https://pkg.go.dev/github.com/golang-migrate/migrate/v4).
The `schema_migrations` table is compatible with this library, so you can change freely back and forth between them.

The goal of this library is to apply migrations atomically in a single transactions.
As a bonus this makes it safe for concurrent use (and there are tests which assert this).

This version uses the `zombiezen.com/go/sqlite` package.

## Example usage

```golang
import (
  "context"

  "zombiezen.com/go/sqlite/sqlitex"
)

//go:embed: migrations/*.sql
var migrations embed.FS

func main() {
  pool, err := sqlitex.Open(":memory:", 0, 10)
  if err != nil {
    panic(err)
  }

  defer pool.Close()
  
  if err := Up(context.Background(), pool); err != nil {
    panic(err)
  }

  // Do something with the db here
}

func Up(ctx context.Context, pool *sqlitex.Pool) error {
  conn := pool.Get(ctx)
  defer pool.Put(conn)

  m, err := migrations.NewMigrationsFromFs(migrations, ".")
  if err != nil {
    return err
  }

  return m.Up(ctx, conn)
}
```
