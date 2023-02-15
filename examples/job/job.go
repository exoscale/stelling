package job

import (
	"context"
	"time"

	"github.com/exoscale/stelling/examples/config"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Dependency is a dummy type that represents a shared component
// that our Job will depend on
type Dependency struct {
	state *time.Ticker
}

// New Dependency adds a *Dependency to the system and registers lifecycle hooks
// This allows the Dependency to manage its own bootstrap and cleanup
// Because constructors are invoked lazily, the lifecycle hook will only execute
// if the system is actually using the component
func NewDependency(lc fx.Lifecycle, conf *config.Config) *Dependency {
	d := &Dependency{
		state: time.NewTicker(conf.Interval),
	}
	lc.Append(fx.Hook{
		OnStop: d.Stop,
	})
	return d
}

func (d *Dependency) Stop(ctx context.Context) error {
	d.state.Stop()
	return nil
}

// Job simulates our top level artifact
// It keeps some state and uses its dependency to execute a side-effect
// We also keep track of the errors that have occured:
// Depending on the job you may want to just report all failures out or
// stop after the first failure
type Job struct {
	d      *Dependency
	logger *zap.Logger
	count  int
	err    *multierror.Error
}

func NewJob(d *Dependency, logger *zap.Logger) *Job {
	return &Job{
		d:      d,
		logger: logger,
	}
}

func (j *Job) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			j.logger.Info("Job was explicitly canceled")
			return
		case <-j.d.state.C:
			// In this example we assume that each iteration is independent
			// We track the errors, but don't exit early
			// If an error is fatal, you can save it on the job and immediately
			// return here
			if err := sideEffect(); err != nil {
				j.err = multierror.Append(j.err, err)
			}
			j.count++
			j.logger.Info("Job progress", zap.Int("count", j.count))
			if j.count == 5 {
				j.logger.Info("Job finished", zap.Int("count", j.count))
				return
			}
		}
	}
}

func sideEffect() error {
	return nil
}

// InvokeJob is the function we'll Invoke in our system
// In its OnStart hook we spawn the go routine that executes the work
// We use an fx.Shutdowner to stop the system when all work is done
// In its OnStop hook, we check if there were any errors and return them:
// this will cause the program to return a non-zero exit code if any errors
// happened during execution
func InvokeJob(lc fx.Lifecycle, sd fx.Shutdowner, job *Job, logger *zap.Logger) {
	jobCtx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				job.Run(jobCtx)
				sd.Shutdown()
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			return job.err.ErrorOrNil()
		},
	})
}
