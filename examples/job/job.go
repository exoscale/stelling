package job

import (
	"context"
	"errors"
	"time"

	"github.com/exoscale/stelling/examples/config"
	"github.com/exoscale/stelling/fxutils"
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
type Job struct {
	d      *Dependency
	logger *zap.Logger
	count  int
}

func NewJob(d *Dependency, logger *zap.Logger) fxutils.Job {
	return &Job{
		d:      d,
		logger: logger,
	}
}

func (j *Job) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			j.logger.Info("Job was explicitly canceled")
			return ctx.Err()
		case <-j.d.state.C:
			// In this example we assume that each iteration is independent
			// We track the errors, but don't exit early
			// If an error is fatal, you can save it on the job and immediately
			// return here
			errs := []error{}
			if err := sideEffect(); err != nil {
				errs = append(errs, err)
			}
			j.count++
			j.logger.Info("Job progress", zap.Int("count", j.count))
			if j.count == 5 {
				j.logger.Info("Job finished", zap.Int("count", j.count))
				return errors.Join(errs...)
			}
		}
	}
}

func sideEffect() error {
	return nil
}
