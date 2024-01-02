package migrationx

import (
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestNewMigrations(t *testing.T) {
	t.Run("Should return an error if up and down migrations do not match", func(t *testing.T) {
		up := []string{"migration1", "migration2"}
		down := []string{"down1"}

		_, err := NewMigrations(up, down)
		require.EqualError(t, err, "Must have a 'down' migration for each 'up' migration")
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
		require.EqualError(t, err, "Target directory must have a 'down' migration for each 'up' migration")
	})

	t.Run("Should return an error if an up migration is missing", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"03_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"02_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		_, err := NewMigrationsFromFS(fsys, ".")
		require.EqualError(t, err, "Up migration for migration 2 is missing")
	})

	t.Run("Should return an error if a down migration is missing", func(t *testing.T) {
		fsys := fstest.MapFS{
			"01_initial.up.sql":        &fstest.MapFile{Data: []byte("my up sql")},
			"01_initial.down.sql":      &fstest.MapFile{Data: []byte("my down sql")},
			"02_modification.up.sql":   &fstest.MapFile{Data: []byte("other sql")},
			"03_modification.down.sql": &fstest.MapFile{Data: []byte("other sql")},
		}

		_, err := NewMigrationsFromFS(fsys, ".")
		require.EqualError(t, err, "Down migration for migration 2 is missing")
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

func TestParseMigration(t *testing.T) {
	cases := []struct {
		input  string
		output *migration
		ok     bool
	}{
		{input: "", output: nil, ok: false},
		{input: "random", output: nil, ok: false},
		{input: "migration.sql", output: nil, ok: false},
		{input: "00_migration.up.sql", output: nil, ok: false},
		{input: "01_migration.up.SQL", output: nil, ok: false},
		{input: "01_migration.UP.sql", output: nil, ok: false},
		{input: "01.migration.up.sql", output: nil, ok: false},
		{input: "0001.migration.up.sql", output: nil, ok: false},
		{input: "migration.up.sql", output: nil, ok: false},
		{input: "0001_migration.up.sql", output: &migration{pos: 1, up: true, name: "0001_migration.up.sql"}, ok: true},
		{input: "0121_migration.up.sql", output: &migration{pos: 121, up: true, name: "0121_migration.up.sql"}, ok: true},
		{input: "01_migration.up.sql", output: &migration{pos: 1, up: true, name: "01_migration.up.sql"}, ok: true},
		{input: "01_migration.down.sql", output: &migration{pos: 1, up: false, name: "01_migration.down.sql"}, ok: true},
		{input: "42_migration.up.sql", output: &migration{pos: 42, up: true, name: "42_migration.up.sql"}, ok: true},
		{input: "01_migration_with_multi.ple-specIAL|characters.up.sql", output: &migration{pos: 1, up: true, name: "01_migration_with_multi.ple-specIAL|characters.up.sql"}, ok: true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			m, ok := parseMigration(tc.input)
			require.Equal(t, tc.output, m)
			require.Equal(t, tc.ok, ok)
		})
	}
}

func testDb(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn, err := sqlite.OpenConn(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func dbSchema(t *testing.T, conn *sqlite.Conn) []string {
	t.Helper()

	statements := []string{}
	require.NoError(t, sqlitex.ExecuteTransient(conn, "SELECT sql FROM sqlite_schema;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			statements = append(statements, stmt.ColumnText(0))
			return nil
		},
	}))

	return statements
}

func TestEnsureVersionSchema(t *testing.T) {
	t.Run("Should apply the version table and index when not present", func(t *testing.T) {
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		}
		conn := testDb(t)

		require.NoError(t, ensureVersionSchema(conn))

		statements := dbSchema(t, conn)

		require.Equal(t, expected, statements)
	})

	t.Run("Should not error if the table already exists", func(t *testing.T) {
		expected := []string{
			"CREATE TABLE schema_migrations (version uint64, dirty bool)",
			"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		}
		conn := testDb(t)

		require.NoError(t, ensureVersionSchema(conn))
		require.NoError(t, ensureVersionSchema(conn))

		statements := dbSchema(t, conn)

		require.Equal(t, expected, statements)
	})
}

func TestDbVersion(t *testing.T) {
	t.Run("Should return an error if the version table has not been provisioned", func(t *testing.T) {
		conn := testDb(t)

		_, err := dbVersion(conn)
		require.Error(t, err)
	})

	t.Run("Should return 0 if no version is set yet", func(t *testing.T) {
		conn := testDb(t)
		require.NoError(t, ensureVersionSchema(conn))

		version, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, uint64(0), version)
	})

	t.Run("Should return the current version", func(t *testing.T) {
		conn := testDb(t)
		require.NoError(t, ensureVersionSchema(conn))
		expected := uint64(42)

		require.NoError(t, sqlitex.ExecuteTransient(
			conn,
			"INSERT INTO schema_migrations (version, dirty) VALUES (?, ?);",
			&sqlitex.ExecOptions{Args: []any{expected, false}},
		))

		version, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, expected, version)
	})
}

func TestSetDbVersion(t *testing.T) {
	t.Run("Should return an error if the version table has not been provisioned", func(t *testing.T) {
		conn := testDb(t)

		require.Error(t, setDbVersion(conn, 12))
	})

	t.Run("Should set a value that will be returned by dbVersion", func(t *testing.T) {
		conn := testDb(t)
		require.NoError(t, ensureVersionSchema(conn))
		expected := uint64(74)

		require.NoError(t, setDbVersion(conn, expected))
		version, err := dbVersion(conn)
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
		conn := testDb(t)

		require.NoError(t, migrations.Migrate(conn, 1))

		statements := dbSchema(t, conn)

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
		conn := testDb(t)

		require.NoError(t, migrations.Migrate(conn, targetVersion))

		statements := dbSchema(t, conn)
		version, err := dbVersion(conn)
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
		conn := testDb(t)

		require.NoError(t, migrations.Migrate(conn, firstVersion))

		v1, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, firstVersion, v1)

		// The idea is that this will error out if it will try to recreate the tables that
		// already exist
		targetVersion := uint64(5)
		require.NoError(t, migrations.Migrate(conn, targetVersion))
		v2, err := dbVersion(conn)
		require.NoError(t, err)
		statements := dbSchema(t, conn)

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
		conn := testDb(t)

		// Let's migrate up to the highest version first
		require.NoError(t, migrations.Migrate(conn, highestVersion))
		v3, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, highestVersion, v3)

		// Now migrate down
		require.NoError(t, migrations.Migrate(conn, targetVersion))

		statements := dbSchema(t, conn)
		version, err := dbVersion(conn)
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
		conn := testDb(t)

		// Let's migrate up to the highest version first
		require.NoError(t, migrations.Migrate(conn, targetVersion))
		v3, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, targetVersion, v3)

		// Migrate to the highest version again
		require.NoError(t, migrations.Migrate(conn, targetVersion))

		statements := dbSchema(t, conn)
		version, err := dbVersion(conn)
		require.NoError(t, err)

		require.Equal(t, expected, statements)
		require.Equal(t, targetVersion, version)
	})

	t.Run("Should error out if target version is higher than max migration version", func(t *testing.T) {
		up := []string{"not run"}
		down := []string{"not run either"}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		conn := testDb(t)

		require.EqualError(t, migrations.Migrate(conn, 2), "migrate failed: target version 2 is higher than max migration version 1")
	})

	t.Run("Should error out if db version is higher than max migration version", func(t *testing.T) {
		up := []string{"not run", "not run"}
		down := []string{"not run either", "not run either"}
		migrations, err := NewMigrations(up, down)
		require.NoError(t, err)

		conn := testDb(t)
		require.NoError(t, ensureVersionSchema(conn))
		require.NoError(t, setDbVersion(conn, 12))

		require.EqualError(t, migrations.Migrate(conn, 2), "migrate failed: database version 12 is higher than max migration version 2")

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
		conn := testDb(t)

		// Let's migrate up to v2 first
		firstVersion := uint64(2)
		require.NoError(t, migrations.Migrate(conn, firstVersion))
		v1, err := dbVersion(conn)
		require.NoError(t, err)
		require.Equal(t, firstVersion, v1)

		// Migrate to the highest version
		targetVersion := uint64(4)
		require.Error(t, migrations.Migrate(conn, targetVersion))

		statements := dbSchema(t, conn)
		version, err := dbVersion(conn)
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
		work := func(conn *sqlite.Conn) {
			defer wg.Done()
			defer func() { _ = conn.Close() }()

			// Wait for the test to shoot the gun
			<-startChan
			require.NoError(t, migrations.Migrate(conn, targetVersion))
		}
		for i := 0; i < 3; i++ {
			wg.Add(1)
			conn, err := sqlite.OpenConn(filepath.Join(dbDir, dbName))
			require.NoError(t, err)
			go work(conn)
		}
		close(startChan)
		wg.Wait()

		// Assert schema
		conn, err := sqlite.OpenConn(filepath.Join(dbDir, dbName))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })

		statements := dbSchema(t, conn)
		version, err := dbVersion(conn)
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
	conn := testDb(t)

	require.NoError(t, migrations.Up(conn))

	statements := dbSchema(t, conn)
	version, err := dbVersion(conn)
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
	conn := testDb(t)

	require.NoError(t, migrations.Up(conn))
	v1, err := dbVersion(conn)
	require.NoError(t, err)
	require.Equal(t, uint64(3), v1)

	require.NoError(t, migrations.Down(conn))

	statements := dbSchema(t, conn)
	version, err := dbVersion(conn)
	require.NoError(t, err)

	require.Equal(t, expected, statements)
	require.Equal(t, uint64(0), version)
}
