package main

import (
	"log"
	"os"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/examples/config"
	"github.com/exoscale/stelling/examples/job"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/exoscale/stelling/fxtracing"
	"go.uber.org/fx"
)

func main() {
	// Immediately log a line to show that we've started
	// This can help debug whether something is failing to start us or whether we are
	// stuck in system startup
	log.Println("starting job")

	// Create the object in which we'll try to load our configuration
	conf := &config.Config{}

	// Load the configuration:
	// It will use a config file, environment variables and cli flags
	if err := sconfig.Load(conf, os.Args); err != nil {
		// Config failed to load or failed validation
		// Error out and let the user know why
		log.Fatal(err)
	}

	app := fx.New(createSystem(conf))

	// Start the application:
	// This will run until we receive a signal to shut down or the job finishes
	// It handles its own errors
	app.Run()
}

// createSystem turns the configuration into a system that can be run
// We extract this in a function, because it allows us to write tests
// that assert that it contains all necessary dependencies
func createSystem(conf *config.Config) fx.Option {
	opts := fx.Options(
		// Insert the configuration as is: this allows other components to reference it
		fx.Supply(conf),
		// Add the modules from stelling:
		// These will insert constructors for the various components,
		// as well as register lifecycle hooks if necessary

		// fxlogging adds a *zap.Logger into the system
		fxlogging.NewModule(conf),
		// fxtracing adds a trace.TraceProvider to the system: this can be used to create
		// top-level spans
		// In case the system uses grpc, middleware will be wired up that traces each request
		// As always in go: the current span can be retrieved from the passed in context
		fxtracing.NewModule(conf),
		// fxsentry adds a *sentry.Client to the system
		// It will also configure the zap DPanic level to emit a sentry
		fxsentry.NewModule(conf),

		// Insert our application components
		fx.Provide(
			job.NewDependency,
			job.NewJob,
		),

		// Invoke functions are run in the order in which they are specified
		fx.Invoke(job.StartJob),
	)

	return opts
}