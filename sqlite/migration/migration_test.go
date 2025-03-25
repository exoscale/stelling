package migration

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestNewMigrations(t *testing.T) {
	t.Run("Should return an error if up and down migrations do not match", func(t *testing.T) {
		up := []string{"migration1", "migration2"}
		down := []string{"down1"}

		_, err := NewMigrations(up, down)
		require.EqualError(t, err, "must have a 'down' migration for each 'up' migration")
	})

	t.Run("Should return non-nil Migrations", func(t *testing.T) {
		up := []string{"migration1", "migration2"}
		down := []string{"down1", "down2"}

		migrations, err := NewMigrations(up, down)

		require.NoError(t, err)
		require.NotNil(t, migrations)
	})
}

func TestNewMigrationsFromFS(t *testing.T) {
	t.Run("Should return an error if up and down migrations do not match", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":      &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":    &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		_, err := NewMigrationsFromFS(fsys, ".")
		require.EqualError(t, err, "target directory must have a 'down' migration for each 'up' migration")
	})

	t.Run("Should return an error if an up migration is missing", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"03_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"02_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		_, err := NewMigrationsFromFS(fsys, ".")
		require.EqualError(t, err, "up migration for migration 2 is missing")
	})

	t.Run("Should return an error if a down migration is missing", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"03_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		_, err := NewMigrationsFromFS(fsys, ".")
		require.EqualError(t, err, "down migration for migration 2 is missing")
	})

	t.Run("Should return non-nil migrations", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"02_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		migrations, err := NewMigrationsFromFS(fsys, ".")
		require.NoError(t, err)
		require.NotNil(t, migrations)
	})

	t.Run("Should accept different names for up and down migrations", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":             &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":           &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql":        &fstest.MapFile{Data: []byte("other sql")},
			"02_undo_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		migrations, err := NewMigrationsFromFS(fsys, ".")
		require.NoError(t, err)
		require.NotNil(t, migrations)
	})

	t.Run("Should ignore files not matching the filename template when parsing", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"02_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
			"random.txt":               &fstest.MapFile{Data: []byte("not sql")},
		}

		migrations, err := NewMigrationsFromFS(fsys, ".")
		require.NoError(t, err)
		require.NotNil(t, migrations)
	})
}

func testDb(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func dbSchema(t *testing.T, db sqlExecutor) []string {
	t.Helper()

	statements := []string{}
	rows, err := db.QueryContext(
		context.Background(),
		"SELECT sql FROM sqlite_schema",
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var stmt string
		require.NoError(t, rows.Scan(&stmt))
		statements = append(statements, stmt)
	}
	require.NoError(t, rows.Err())

	return statements
}

func TestEnsureVersionSchema(t *testing.T) {
	t.Run("Should apply the version table and index when not present", func(t *testing.T) {
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		}
		db := testDb(t)
		ctx := context.Background()

		require.NoError(t, ensureVersionSchema(ctx, db))

		statements := dbSchema(t, db)

		require.Equal(t, expected, statements)
	})

	t.Run("Should not error if the table already exists", func(t *testing.T) {
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		}
		db := testDb(t)
		ctx := context.Background()

		require.NoError(t, ensureVersionSchema(ctx, db))
		require.NoError(t, ensureVersionSchema(ctx, db))

		statements := dbSchema(t, db)

		require.Equal(t, expected, statements)
	})
}

func TestDbVersion(t *testing.T) {
	t.Run("Should return an error if the version table has not been provisioned", func(t *testing.T) {
		db := testDb(t)

		_, err := dbVersion(context.Background(), db)
		require.Error(t, err)
	})

	t.Run("Should return 0 if no version is set yet", func(t *testing.T) {
		db := testDb(t)
		ctx := context.Background()
		require.NoError(t, ensureVersionSchema(ctx, db))

		version, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, uint64(0), version)
	})

	t.Run("Should return the current version", func(t *testing.T) {
		db := testDb(t)
		ctx := context.Background()
		require.NoError(t, ensureVersionSchema(ctx, db))
		expected := uint64(42)

		_, err := db.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)",
			expected,
			false,
		)
		require.NoError(t, err)

		version, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, expected, version)
	})
}

func TestSetDbVersion(t *testing.T) {
	t.Run("Should return an error if the version table has not been provisioned", func(t *testing.T) {
		db := testDb(t)

		require.Error(t, setDbVersion(context.Background(), db, 12))
	})

	t.Run("Should set a value that will be returned by dbVersion", func(t *testing.T) {
		db := testDb(t)
		ctx := context.Background()
		require.NoError(t, ensureVersionSchema(ctx, db))
		expected := uint64(74)

		require.NoError(t, setDbVersion(ctx, db, expected))
		version, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, expected, version)
	})
}

func TestMigrationsMigrate(t *testing.T) {
	t.Run("Should create the version table", func(t *testing.T) {
		up := []string{"CREATE TABLE test (name text, value int);"}
		down := []string{"DROP TABLE test;"}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		// Can't really use versionSchema variable because sqlite does not store the
		// IF NOT EXISTS directive
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test (name text, value int)",
		}
		db := testDb(t)

		require.NoError(t, migrations.Migrate(context.Background(), db, 1))

		statements := dbSchema(t, db)

		require.Equal(t, expected, statements)
	})

	t.Run("Should migrate up to the given version", func(t *testing.T) {
		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
		}
		targetVersion := uint64(2)
		db := testDb(t)
		ctx := context.Background()

		require.NoError(t, migrations.Migrate(ctx, db, targetVersion))

		statements := dbSchema(t, db)
		version, err := dbVersion(ctx, db)
		require.NoError(t, err)

		require.Equal(t, expected, statements)
		require.Equal(t, targetVersion, version)
	})

	t.Run("Should only apply the minimal set of migrations between versions", func(t *testing.T) {
		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
			"CREATE TABLE test4 (name text, value int);",
			"CREATE TABLE test5 (name text, value int);",
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
			"DROP TABLE test4;",
			"DROP TABLE test5;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
			"CREATE TABLE test3 (name text, value int)",
			"CREATE TABLE test4 (name text, value int)",
			"CREATE TABLE test5 (name text, value int)",
		}
		firstVersion := uint64(2)
		db := testDb(t)
		ctx := context.Background()

		require.NoError(t, migrations.Migrate(ctx, db, firstVersion))

		v1, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, firstVersion, v1)

		// The idea is that this will error out if it will try to recreate the tables that
		// already exist
		targetVersion := uint64(5)
		require.NoError(t, migrations.Migrate(context.Background(), db, targetVersion))
		v2, err := dbVersion(ctx, db)
		require.NoError(t, err)
		statements := dbSchema(t, db)

		require.Equal(t, targetVersion, v2)
		require.Equal(t, expected, statements)
	})

	t.Run("Should migrate down to the given version", func(t *testing.T) {
		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
			"CREATE TABLE test4 (name text, value int);",
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
			"DROP TABLE test4;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
		}
		targetVersion := uint64(2)
		highestVersion := uint64(4)
		db := testDb(t)
		ctx := context.Background()

		// Let's migrate up to the highest version first
		require.NoError(t, migrations.Migrate(ctx, db, highestVersion))
		v3, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, highestVersion, v3)

		// Now migrate down
		require.NoError(t, migrations.Migrate(ctx, db, targetVersion))

		statements := dbSchema(t, db)
		version, err := dbVersion(ctx, db)
		require.NoError(t, err)

		require.Equal(t, expected, statements)
		require.Equal(t, targetVersion, version)
	})

	t.Run("Should not modify the schema if versions match", func(t *testing.T) {
		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
			"CREATE TABLE test3 (name text, value int)",
		}
		targetVersion := uint64(3)
		db := testDb(t)
		ctx := context.Background()

		// Let's migrate up to the highest version first
		require.NoError(t, migrations.Migrate(ctx, db, targetVersion))
		v3, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, targetVersion, v3)

		// Migrate to the highest version again
		require.NoError(t, migrations.Migrate(ctx, db, targetVersion))

		statements := dbSchema(t, db)
		version, err := dbVersion(ctx, db)
		require.NoError(t, err, statements)

		require.Equal(t, expected, statements)
		require.Equal(t, targetVersion, version)
	})

	t.Run("Should error out if target version is higher than max migration version", func(t *testing.T) {
		up := []string{"not run"}
		down := []string{"not run either"}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		db := testDb(t)

		require.EqualError(t, migrations.Migrate(context.Background(), db, 2), "migrate failed: target version 2 is higher than max migration version 1")
	})

	t.Run("Should error out if db version is higher than max migration version", func(t *testing.T) {
		up := []string{"not run", "not run"}
		down := []string{"not run either", "not run either"}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		db := testDb(t)
		ctx := context.Background()
		require.NoError(t, ensureVersionSchema(ctx, db))
		require.NoError(t, setDbVersion(ctx, db, 12))

		require.EqualError(t, migrations.Migrate(ctx, db, 2), "migrate failed: database version 12 is higher than max migration version 2")

	})

	t.Run("Should roll back all modifications if the migration errors out", func(t *testing.T) {
		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);", // This will fail because it already exists
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
			"DROP TABLE test4;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
			// We expect the creation of table3 to be rolled back
		}
		db := testDb(t)
		ctx := context.Background()

		// Let's migrate up to v2 first
		firstVersion := uint64(2)
		require.NoError(t, migrations.Migrate(ctx, db, firstVersion))
		v1, err := dbVersion(ctx, db)
		require.NoError(t, err)
		require.Equal(t, firstVersion, v1)

		// Migrate to the highest version
		targetVersion := uint64(4)
		require.Error(t, migrations.Migrate(ctx, db, targetVersion))

		statements := dbSchema(t, db)
		version, err := dbVersion(ctx, db)
		require.NoError(t, err)

		require.Equal(t, expected, statements)
		require.Equal(t, firstVersion, version)
	})

	t.Run("Should support concurrent version migrations", func(t *testing.T) {
		// The whole reason we wrote this code in the first place

		// We'll create a number of goroutines that all try to migrate the database to the same version
		// We will synchronise their start on a goroutine to maximise the concurrency
		// We expect all of them to succeed:
		// * 1 will apply the schema
		// * The others will retry and observe the database after this change and noop out

		up := []string{
			"CREATE TABLE test1 (name text, value int);",
			"CREATE TABLE test2 (name text, value int);",
			"CREATE TABLE test3 (name text, value int);",
			"CREATE TABLE test4 (name text, value int);",
		}
		down := []string{
			"DROP TABLE test1;",
			"DROP TABLE test2;",
			"DROP TABLE test3;",
			"DROP TABLE test4;",
		}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)
		targetVersion := uint64(4)
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
			"CREATE TABLE test1 (name text, value int)",
			"CREATE TABLE test2 (name text, value int)",
			"CREATE TABLE test3 (name text, value int)",
			"CREATE TABLE test4 (name text, value int)",
		}

		// In memory db won't cut it, because we want to connect as if we're different processes
		dbDir := t.TempDir()
		dbName := "testdb"

		startChan := make(chan any)
		wg := &sync.WaitGroup{}
		work := func(db *sql.DB) {
			defer wg.Done()
			defer func() { _ = db.Close() }()

			// Wait for the test to shoot the gun
			<-startChan
			require.NoError(t, migrations.Migrate(context.Background(), db, targetVersion), "iteration %d")
		}
		for i := 0; i < 10; i++ {
			wg.Add(1)
			db, err := sql.Open("sqlite", filepath.Join(dbDir, dbName)+"?_pragma=busy_timeout(5000)")
			require.NoError(t, err)
			go work(db)
		}
		close(startChan)
		wg.Wait()

		// Assert schema
		db, err := sql.Open("sqlite", filepath.Join(dbDir, dbName))
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		statements := dbSchema(t, db)
		version, err := dbVersion(context.Background(), db)
		require.NoError(t, err)

		require.Equal(t, targetVersion, version)
		require.Equal(t, expected, statements)
	})
}

func TestMigrationsUp(t *testing.T) {
	up := []string{
		"CREATE TABLE test1 (name text, value int);",
		"CREATE TABLE test2 (name text, value int);",
		"CREATE TABLE test3 (name text, value int);",
	}
	down := []string{
		"DROP TABLE test1;",
		"DROP TABLE test2;",
		"DROP TABLE test3;",
	}
	migrations, err := NewMigrations(up, down)
	require.NoError(t, err)

	expected := []string{
		"CREATE TABLE schema_migrations (version uint64, dirty bool)",
		"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		"CREATE TABLE test1 (name text, value int)",
		"CREATE TABLE test2 (name text, value int)",
		"CREATE TABLE test3 (name text, value int)",
	}
	db := testDb(t)
	ctx := context.Background()

	require.NoError(t, migrations.Up(ctx, db))

	statements := dbSchema(t, db)
	version, err := dbVersion(ctx, db)
	require.NoError(t, err)

	require.Equal(t, expected, statements)
	require.Equal(t, uint64(3), version)
}

func TestMigrationsDown(t *testing.T) {
	up := []string{
		"CREATE TABLE test1 (name text, value int);",
		"CREATE TABLE test2 (name text, value int);",
		"CREATE TABLE test3 (name text, value int);",
	}
	down := []string{
		"DROP TABLE test1;",
		"DROP TABLE test2;",
		"DROP TABLE test3;",
	}
	migrations, err := NewMigrations(up, down)
	require.NoError(t, err)

	expected := []string{
		"CREATE TABLE schema_migrations (version uint64, dirty bool)",
		"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
	}
	db := testDb(t)
	ctx := context.Background()

	require.NoError(t, migrations.Up(ctx, db))
	v1, err := dbVersion(ctx, db)
	require.NoError(t, err)
	require.Equal(t, uint64(3), v1)

	require.NoError(t, migrations.Down(ctx, db))

	statements := dbSchema(t, db)
	version, err := dbVersion(ctx, db)
	require.NoError(t, err)

	require.Equal(t, expected, statements)
	require.Equal(t, uint64(0), version)
}
