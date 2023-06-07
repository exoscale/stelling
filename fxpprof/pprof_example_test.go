package fxpprof_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxpprof"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func Example_server() {
	type Config struct {
		fxpprof.Pprof
	}

	conf := &Config{}
	args := []string{"pprof-server", "--pprof.enabled", "-pprof.server.address", "localhost:8080"}
	if err := sconfig.Load(conf, args); err != nil {
		fmt.Println(err)
		return
	}

	run := func(lc fx.Lifecycle, sd fx.Shutdowner) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					// By default the pprof server binds to localhost:9092
					resp, err := http.DefaultClient.Get("http://localhost:9092/debug/pprof") //nolint:noctx
					if err != nil {
						panic(err)
					}
					defer resp.Body.Close()
					fmt.Println("Response code for GET http://localhost:9092/debug/pprof:", resp.StatusCode)
					sd.Shutdown() //nolint:errcheck
				}()
				return nil
			},
		})
	}

	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Provide(zap.NewNop),
		fxpprof.NewModule(conf),
		fx.Invoke(run),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Response code for GET http://localhost:9092/debug/pprof: 200
}

func Example_job() {
	type Config struct {
		fxpprof.Pprof
	}

	tmp, err := os.MkdirTemp("", "pprof")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	conf := &Config{}
	// By setting GenerateFiles, we instruct the module to profile the entire
	// process runtime and output the profiles in the given directory
	args := []string{"pprof-job", "--pprof.generate-files", tmp}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	run := func(lc fx.Lifecycle, sd fx.Shutdowner) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					// Sleeping for a bit here to give the profiler something to capture
					<-time.After(10 * time.Millisecond)
					sd.Shutdown() //nolint:errcheck
				}()
				return nil
			},
		})
	}

	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxpprof.NewModule(conf),
		fx.Invoke(run),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	app := fx.New(opts)
	app.Run()

	// Assert that the target files exist and print something if everything is fine
	if _, err := os.Stat(filepath.Join(tmp, "pprof.cpu")); err != nil {
		panic(err)
	}
	fmt.Println("pprof.cpu exists in the given directory")
	if _, err := os.Stat(filepath.Join(tmp, "pprof.mem")); err != nil {
		panic(err)
	}
	fmt.Println("pprof.mem exists in the given directory")

	// Output:
	// pprof.cpu exists in the given directory
	// pprof.mem exists in the given directory
}
