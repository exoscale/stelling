# Sqlite Module

This module provides support sqlite using the stdlib `sql.DB` and [gorm](https://gorm.io).

It will provide a `*sql.DB` to the database with sane default values. If your application uses gorm,
you can pull out an `*gorm.DB` using the same underlying `*sql.DB` instead.
Furthermore it is capable of running a schema migration at system startup using stelling [migration
package](https://pkg.go.dev/exoscale/stelling/sqlite/migration).

> This module is great for simple applications, but the stdlib sql library has limitations.
If you bump into them you can look at the fxsqlitex package, which has the same module API.

## Components

The module lazily provides the following components:

* A `*sql.DB`
* A `*gorm.DB`

The outputs are marked as private, to make it easy to embed a sqlite database in a module without
having to resort to named components.

## Options

The module provides the following options:

* WithMigrations and WithSchemaVersion
  This allows you to pass in a schema migration that will be run at system startup
* WithSQLiteOptions
  These options will be passed to the `*sql.DB` and allow configuring some of its behaviour

## Configuration file

The embeddable configuration only exposes a single option for the moment:

* `path`: The filesystem path where the database file should be created or can be opened
