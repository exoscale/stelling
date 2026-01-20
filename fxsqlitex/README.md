# Sqlitex Module

This module provides [sqlitex](https://pkg.go.dev/zombiezen.com/go/sqlite/sqlitex) support.

It will provide a connection pool to the database with sane default values.
Furthermore it is capable of running a schema migration at system startup using stelling [migrationx
package](https://pkg.go.dev/exoscale/stelling/sqlite/migrationx).

> This is an advanced high performance driver for sqlite. While it has excellent runtime
characteristics, writing queries is very verbose and cumbersome. For simple applications you most
likely want to use the fxsqlite module, which provides an stdlb `sql.DB` and enables the use higher
level libraries gorm. The API of both modules has been explicitly kept in sync to make it easy to
change between them if the needs of your application change.

## Components

The module lazily provides the following components:

* A `*sqlitex.Pool`

The pool is marked as private, to make it easy to embed a sqlite database in a module without
having to resort to named components.

## Options

The module provides the following options:

* WithMigrations and WithSchemaVersion
  This allows you to pass in a schema migration that will be run at system startup
* WithSQLiteOptions
  These options will be passed to the `*sqlitex.Pool` and allow configuring some of its behaviour

## Configuration file

The embeddable configuration only exposes a single option for the moment:

* `path`: The filesystem path where the database file should be created or can be opened
