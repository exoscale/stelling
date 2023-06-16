package fxhttp_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

type Config struct {
	fxhttp.Server
}

func Example() {
	conf := &Config{}
	args := []string{"http-test"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxhttp.NewModule(conf),
		fx.Provide(
			// supplying a NopLogger to make output deterministic
			// In practise you'd use fxlogging.NewModule to get a zap logger
			// and replace the fxevent logger
			zap.NewNop,
			newMux,
		),
		fx.Invoke(registerHandler),
		// We explicitly need to invoke this, because ordering matters
		fx.Invoke(fxhttp.StartHttpServer),
		fx.Invoke(run),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}

	fx.New(opts).Run()

	// Output:
	// Response code for GET http://localhost:8080/foo 200
	// Endpoint returned correct data true
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/foo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello Stelling"))
	})
	return mux
}

func registerHandler(s *http.Server, mux *http.ServeMux) {
	// You probably want to request a slice of routes here
	// And then create the mux in this function and set it to the server
	s.Handler = mux
}

func run(lc fx.Lifecycle, sd fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				resp, err := http.DefaultClient.Get("http://localhost:8080/foo") //nolint:noctx
				if err != nil {
					panic(err)
				}
				fmt.Println("Response code for GET http://localhost:8080/foo", resp.StatusCode)
				body := resp.Body
				data, err := io.ReadAll(body)
				if err != nil {
					panic(err)
				}
				hasData := bytes.Equal(data, []byte("Hello Stelling"))
				fmt.Println("Endpoint returned correct data", hasData)
				defer body.Close()
				sd.Shutdown() //nolint:errcheck
			}()
			return nil
		},
	})
}
