# Fxutils

As with all things called `utils`, this contains a grabbag of (hopefully) useful things that are
typically too generic to fall into any other category.

## RunWithSystem

`RunWithSystem` tries to simplify running a Job with fx: Run a function until the end and then
terminate the process.

Fx is generally designed to run long running daemons, however there are plenty of reasons you may
want to reuse the modules of your daemon in a job:

* CLI and debug tooling
* Ad hoc jobs to fix issues with the system
* Regularly scheduled cronjobs

While you can absolute run a job with Fx, the plumbing is a bit cumbersome. Fortunately it is also
generic and that is where `RunWithSystem` comes in.

`RunWithSystem` takes as input an `fx` system, and will execute the `Run` method of the
`fxutils.Job` it contains.
The system must `Provide` exactly 1 `Job`.
The process will terminate when the `Run` function returns. If `Run` returns a non-nil error, the
process will return a non-zero exit code.

> Note that it is important the constructor of your Job returns the fxutils.Job type and not the
concrete type, because fx is not capable of doing the type inference automatically.

A simple example would be:

```go
package main

import (
  "context"
  
  "github.com/exoscale/stelling/fxutils"
  "go.uber.org/fx"
)

type MyJob struct {
  Dep1 *Dependency1
  Dep2 *Dependency2
}

// It is important the constructor returns an fxutils.Job and not the concrete type.
func NewMyJob(dep1 *Dependency1, dep2 *Dependency2) fxutils.Job {
  return &MyJob{
    Dep1: dep1,
    Dep2: dep2,
  }
}

// Run is the main entrypoint of your job
// It is given a context that will cancel if the user sends SIGTERM to the process, allowing
// you to gracefully shutdown
func (j *MyJob) Run(ctx context.Context) error {
  // Do things
  return nil
}

func main() {
  system := fx.Options(
    fx.Provide(
      NewDependency1,
      NewDependency2,
      NewMyJob,
    )
  )

  fxutils.RunWithSystem(system)
}
```
