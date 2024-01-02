package migrationx

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const VersionSchema string = `CREATE TABLE IF NOT EXISTS schema_migrations (version uint64, dirty bool);
CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON schema_migrations (version);`

func ensureVersionSchema(conn *sqlite.Conn) (err error) {
	defer sqlitex.Save(conn)(&err)

	return sqlitex.ExecuteScript(conn, VersionSchema, &sqlitex.ExecOptions{})
}

func dbVersion(conn *sqlite.Conn) (uint64, error) {
	var version uint64
	err := sqlitex.ExecuteTransient(
		conn,
		"SELECT version FROM schema_migrations LIMIT 1;",
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				version = uint64(stmt.ColumnInt64(0))
				return nil
			},
		},
	)
	return version, err
}

func setDbVersion(conn *sqlite.Conn, version uint64) error {
	if err := sqlitex.ExecuteTransient(conn, "DELETE FROM schema_migrations;", nil); err != nil {
		return err
	}
	return sqlitex.ExecuteTransient(
		conn,
		"INSERT INTO schema_migrations (version, dirty) VALUES (?, ?);",
		&sqlitex.ExecOptions{Args: []any{version, false}},
	)
}

type Migrations struct {
	UpScripts   []string
	DownScripts []string
}

func NewMigrations(up []string, down []string) (*Migrations, error) {
	if len(up) != len(down) {
		return nil, fmt.Errorf("Must have a 'down' migration for each 'up' migration")
	}

	return &Migrations{
		UpScripts:   up,
		DownScripts: down,
	}, nil
}

type migration struct {
	pos  uint64
	up   bool
	name string
}

type migrationList []*migration

func (m migrationList) Len() int           { return len(m) }
func (m migrationList) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m migrationList) Less(i, j int) bool { return m[i].pos < m[j].pos }

func NewMigrationsFromFS(fsys fs.FS, subpath string) (*Migrations, error) {
	entries, err := fs.ReadDir(fsys, subpath)
	if err != nil {
		return nil, err
	}
	upFiles := make([]*migration, 0, len(entries))
	downFiles := make([]*migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if migration, ok := parseMigration(entry.Name()); ok {
			if migration.up {
				upFiles = append(upFiles, migration)
			} else {
				downFiles = append(downFiles, migration)
			}
		}
	}
	if len(upFiles) != len(downFiles) {
		return nil, fmt.Errorf("Target directory must have a 'down' migration for each 'up' migration")
	}
	sort.Sort(migrationList(upFiles))
	sort.Sort(migrationList(downFiles))
	for i, m := range upFiles {
		if i != int(m.pos)-1 {
			return nil, fmt.Errorf("Up migration for migration %d is missing", i+1)
		}
	}
	for i, m := range downFiles {
		if i != int(m.pos)-1 {
			return nil, fmt.Errorf("Down migration for migration %d is missing", i+1)
		}
	}
	output := &Migrations{
		UpScripts:   make([]string, len(upFiles)),
		DownScripts: make([]string, len(downFiles)),
	}
	for i := range upFiles {
		if content, err := readString(fsys, subpath, upFiles[i].name); err != nil {
			return nil, err
		} else {
			output.UpScripts[i] = content
		}
		if content, err := readString(fsys, subpath, downFiles[i].name); err != nil {
			return nil, err
		} else {
			output.DownScripts[i] = content
		}
	}
	return output, nil
}

var filenameRegex = regexp.MustCompile(`([0-9]+)_.*\.(up|down)\.sql`)

func parseMigration(name string) (*migration, bool) {
	matches := filenameRegex.FindStringSubmatch(name)
	if len(matches) != 3 {
		return nil, false
	}
	pos, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil || pos == 0 {
		return nil, false
	}
	scriptType := matches[2]
	if scriptType != "up" && scriptType != "down" {
		return nil, false
	}
	return &migration{pos: pos, up: scriptType == "up", name: name}, true
}

func readString(fsys fs.FS, subpath string, filename string) (string, error) {
	f, err := fsys.Open(filepath.Join(subpath, filename))
	if err != nil {
		return "", err
	}
	content := new(strings.Builder)
	_, err = io.Copy(content, f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %w", filename, err)
	}
	return content.String(), nil
}

func (m *Migrations) Up(conn *sqlite.Conn) (err error) {
	targetVersion := uint64(len(m.UpScripts))
	return m.Migrate(conn, targetVersion)
}

func (m *Migrations) Down(conn *sqlite.Conn) (err error) {
	return m.Migrate(conn, 0)
}

func (m *Migrations) Migrate(conn *sqlite.Conn, targetVersion uint64) (err error) {
	defer sqlitex.Save(conn)(&err)

	if uint64(len(m.UpScripts)) < targetVersion {
		return fmt.Errorf("migrate failed: target version %d is higher than max migration version %d", targetVersion, len(m.UpScripts))
	}

	if err := ensureVersionSchema(conn); err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}

	version, err := dbVersion(conn)
	if err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}

	if version == targetVersion {
		return nil
	}

	if uint64(len(m.UpScripts)) < version {
		return fmt.Errorf("migrate failed: database version %d is higher than max migration version %d", version, len(m.UpScripts))
	}

	if targetVersion < version {
		for i := int(version - 1); i >= int(targetVersion); i-- {
			if err := sqlitex.ExecuteScript(conn, m.DownScripts[i], nil); err != nil {
				return fmt.Errorf("migrate failed: %w", err)
			}
		}
	} else {
		for _, migration := range m.UpScripts[version:targetVersion] {
			if err := sqlitex.ExecuteScript(conn, migration, nil); err != nil {
				return fmt.Errorf("migrate failed: %w", err)
			}
		}
	}

	if err := setDbVersion(conn, targetVersion); err != nil {
		return fmt.Errorf("migrate failed: %w", err)
	}
	return nil
}
