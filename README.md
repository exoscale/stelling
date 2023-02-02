# Stelling

A collection of opinionated golang scaffolding packages.

The various packages in this module can be used standalone, but are optimised to work together, to make it easy to create long running processes in Go.

Detailed documentation for each package is included in the package README.

## Basic Usage
The `examples` directory contains an example project that can be used to bootstrap your own projects.
It contains two entrypoints: a long running daemon and a long running job.

It has been commented extensively and should provide a good starting point if you are looking for tutorial-like content.

## Rationale
We are looking for the following features:
* Provide reusable components that embed our opinions and best practises
* Provide consistent configuration for these components: eg all http server configurations will look and behave exactly the same
* Provide a consistent way to wire up services

The project has the following anti-goals:
* Provide an abstraction over the underlying components.
  What this means in practise is that we intent to provide you with, for example, a standard library http server and not a custom type.
  The focus is on common configuration of known to work well libraries.
* Support human friendly CLI tools.
  We optimize for long running daemons and jobs, not human facing tools.
  This does not mean the packages here can't be used inside of CLI tools, they can and have, but when faced with a choice we will chose
  to optimize for the former.

The majority of the modules are meant to be used with the [fx package](https://pkg.go.dev/go.uber.org/fx).
`fx` is DI library, which serves a similar purpose to `component` in clojure.

It is recommended to at least go through the basic documentation of `fx` at https://uber-go.github.io/fx/get-started/ to get familiar with the concepts.

While the packages in stelling will work better with `fx`, the modules (and `fx` itself) have been designed to make the dependency on `fx` optional:
* All component start logic is typically captured in a `Start` function
* All component teardown logic is typically captured in a `Stop` function
* Components that don't need lifecycle hooks won't have any `fx` types in their signatures
* Components that do need lifecycle hooks will have two constructors:
  - `NewMyType` which is `fx` type free, and returns a `MyType`, leaving the handling of the lifecycle to the caller
  - `ProvideMyType`, which will have `fx` types in its signature and provides wiring to let `fx` handle the lifecycle.

This allows the components to be used in as many situations as possible.