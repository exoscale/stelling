# Sentry Module

This module provides [sentry](https://pkg.go.dev/github.com/getsentry/sentry-go) support.

## Components
The module adds support for sentry in two ways:

* It configures the zap logger to emit sentries on DPanic and Panic.
  The error is mapped to the sentry exception.
  The stacktrace is from the place where DPanic is invoked (go errors do not contain stacktraces)
  Any additional structured data on the log is added as "extra" data to the sentry
* It makes a `*sentry.Client` available to manually create sentries.
  This can be useful if more control over the shape of the sentry is required than what the zap
  integration provides.
  Because the client is not created when no DSN is given, any component requiring a `*sentry.Client`
  must annotate it as optional in fx, and do the necessary `nil` checks.
  See the included example for details.

## Configuration
The module provides the following configuration options:
* `Dsn`: The sentry DSN. The module is disabled when it is `""`
* `Environment`: The value of the environment field in the generated sentries. Defaults to `production`
* `Debug`: Determines whether the sentry client emits debug logs.
* `Process`: The value of the `process` tag of the generated events. Will default to the current binary
  filename if not set.