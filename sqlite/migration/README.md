# Sqlite Migration

Provides a small utility package which performs SQLite database migrations in a single atomic transaction.

The API is modeled on [golang-migrate](https://pkg.go.dev/github.com/golang-migrate/migrate/v4).
The `schema_migrations` table is compatible with this library, so you can change freely back and forth between them.

The goal of this library is to apply migrations atomically in a single transactions.
As a bonus this makes it safe for concurrent use (and there are tests which assert this).

This version uses the stdlib `database/sql` package.

## Example usage

```golang
import (
  "database/sql"
  "context"

  _ "modernc.org/sqlite"
)

//go:embed: migrations/*.sql
var migrations embed.FS

func main() {
  db, err := sql.Open("sqlite", ":memory:")
  if err != nil {
    panic(err)
  }
  defer db.Close()
  
  m, err := migrations.NewMigrationsFromFs(migrations, ".")
  if err != nil {
    panic(err)
  }

  if err := m.Up(ctx, conn); err != nil {
    panic(err)
  }

  // Do something with the db here
}
```
