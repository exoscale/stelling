package fxhttp_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxlogging"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	fxlogging.Logging
	fxhttp.Server
}

func Example() {
	conf := &Config{}
	args := []string{"http-test", "--logging.mode", "production", "--server.port", "8080"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		fxlogging.NewModule(conf),
		fxhttp.NewModule(conf),
		// zapOpts contains options to make the logs determistic so we can test the output
		fx.Supply(fx.Annotate(zapOpts, fx.ResultTags(`group:"zap_opts,flatten"`))),
		fx.Provide(newMux),
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
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"Mode":"production","Port":8080,"TLS":false,"CertFile":"","KeyFile":"","ClientCAFile":""}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"Mode":"production","Port":8080,"TLS":false,"CertFile":"","KeyFile":"","ClientCAFile":""}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Starting http server","port":8080}
	// Response code for GET http://localhost:8080/foo 200
	// Endpoint returned correct data true
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Stopping http server"}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Done serving http"}
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
				resp, err := http.DefaultClient.Get("http://localhost:8080/foo")
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

var zapOpts = []zap.Option{
	zap.WithCaller(false),
	zap.WithClock(&fixedClock{ts: 1257894000}),
}

type fixedClock struct {
	ts int64
}

func (c *fixedClock) Now() time.Time {
	return time.Unix(c.ts, 0).UTC()
}

func (c *fixedClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}
