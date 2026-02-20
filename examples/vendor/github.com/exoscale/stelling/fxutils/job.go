package fxutils

import (
	"context"

	"go.uber.org/fx"
)

// Job is the interface used by `RunWithSystem`
type Job interface {
	// Run executes the job and is given a context that cancels when the user tries to terminate the process
	Run(ctx context.Context) error
}

// RunWithSystem will start the given fx system, look for a `Job` within it and start its `Run` method.
// It takes care of the full system lifecycle and will exit the process once finished.
// If the system does not provide a `Job`, the process will be terminated with a non zero exit code
// and an error will be printed.
func RunWithSystem(opts fx.Option) {
	system := fx.Options(
		opts,
		fx.Invoke(RunJob),
	)

	fx.New(system).Run()
}

// RunJob registers an OnStart Hook that executes a `Job`.
// The Job's `Run` method is called with a context that gets canceled if the user terminates the process.
// It is meant to be inserted as an Invoke function into an fx system.
func RunJob(lc fx.Lifecycle, sd fx.Shutdowner, job Job) {
	ctx, cancel := context.WithCancel(context.Background())
	var err error
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				err = job.Run(ctx)
				if err := sd.Shutdown(); err != nil {
					// We failed to shut down, panic
					panic(err)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return err
		},
	})
}
