package fxutils

import (
	"context"

	"go.uber.org/fx"
)

type Job interface {
	Run(ctx context.Context) error
}

func RunWithSystem(opts fx.Option) {
	system := fx.Options(
		opts,
		fx.Invoke(func(lc fx.Lifecycle, sd fx.Shutdowner, job Job) {
			ctx, cancel := context.WithCancel(context.Background())
			var err error
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						err = job.Run(ctx)
						sd.Shutdown()
					}()
					return nil
				},
				OnStop: func(context.Context) error {
					cancel()
					return err
				},
			})
		}),
	)

	fx.New(system).Run()
}
