package main

import (
	"log"
	"os"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/examples/config"
	"github.com/exoscale/stelling/examples/server"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxgrpc/health"
	"github.com/exoscale/stelling/fxgrpc/reflection"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/exoscale/stelling/fxpprof"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/exoscale/stelling/fxtracing"
	"go.uber.org/fx"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
)

func main() {
	// Immediately log a line to show that we've started
	// This can help debug whether something is failing to start us or whether we are
	// stuck in system startup
	log.Println("starting server")

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
	// This will run until we receive a signal to shut down
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

		// fxpprof starts a pprof server on its own http-server
		// pprof instruments the go runtime directly, you do not have to write any more code
		fxpprof.NewModule(conf),
		// fxmetrics adds a *prometheus.Registry to the system that you can register
		// custom metrics on
		// It also starts its own http-server to expose the prometheus endpoint
		// In case the system uses grpc, it will wire up middleware and expose grpc
		// metrics, such as request counts
		fxmetrics.NewModule(conf),
		// fxtracing adds a trace.TraceProvider to the system: this can be used to create
		// top-level spans
		// In case the system uses grpc, middleware will be wired up that traces each request
		// As always in go: the current span can be retrieved from the passed in context
		fxtracing.NewModule(conf),
		// fxsentry adds a *sentry.Client to the system
		// It will also configure the zap DPanic level to emit a sentry
		fxsentry.NewModule(conf),
		// fxgrpc.ServerModule provides a grpc.Server to the system
		// You generally do not reference this yourself, but add the grpc generated
		// RegisterMyService function to the Invoke list
		fxgrpc.NewServerModule(conf),
		// Add a basic grpc healthcheck service
		// It will automatically register itself on the system grpc.Server
		health.Module,
		// Add the grpc reflection service
		// It will automatically register itself on the system grpc.Server
		reflection.Module,

		// Insert our application components
		fx.Provide(
			server.NewServer,
		),

		// Invoke functions are run in the order in which they are specified
		fx.Invoke(
			// Register the RouteGuideServer on the GrpcServer
			// This will materialize the dependencies from the lazy constructors
			// and causes our system to do something meaningful
			pb.RegisterRouteGuideServer,
			// Invoke the grpc server
			fxgrpc.StartGrpcServer,
		),
	)

	return opts
}