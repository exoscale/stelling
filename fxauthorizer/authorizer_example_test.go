package fxauthorizer_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxauthorizer"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxgrpc/health"
	"github.com/exoscale/stelling/fxhttp"
	"github.com/go-jose/go-jose/v4"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GrpcConfig struct {
	fxgrpc.Server
	fxgrpc.Client
	fxauthorizer.Authorizer
}

type HttpConfig struct {
	fxhttp.Server
	fxauthorizer.Authorizer
}

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func NewRouteGuideServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

// testIDP is a minimal OIDC identity provider used to demonstrate JWT token extraction below
type testIDP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newTestIDP() *testIDP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	idp := &testIDP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":"%s","jwks_uri":"%s/jwks"}`, idp.server.URL, idp.server.URL)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwk := jose.JSONWebKey{Key: idp.key.Public(), Algorithm: string(jose.RS256)}
		b, err := jwk.MarshalJSON()
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(w, `{"keys":[%s]}`, b)
	})
	idp.server = httptest.NewServer(mux)
	return idp
}

// signToken produces a signed JWT with the given subject and groups, issued by this IDP
func (idp *testIDP) signToken(subject string, groups []string) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: idp.key}, nil)
	if err != nil {
		panic(err)
	}
	claims, err := json.Marshal(struct {
		Issuer  string   `json:"iss"`
		Subject string   `json:"sub"`
		Groups  []string `json:"groups"`
		Expiry  int64    `json:"exp"`
	}{
		Issuer:  idp.server.URL,
		Subject: subject,
		Groups:  groups,
		Expiry:  time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		panic(err)
	}
	jws, err := signer.Sign(claims)
	if err != nil {
		panic(err)
	}
	token, err := jws.CompactSerialize()
	if err != nil {
		panic(err)
	}
	return token
}

func Example_grpc() {
	idp := newTestIDP()
	defer idp.server.Close()

	// Grpc metadata keys are always lowercased, so we point the extractor at "authorization" rather than the HTTP-style "Authorization"
	conf := &GrpcConfig{}
	rule := "request.service == 'grpc.health.v1.Health' && 'trusted-group' in request.jwt.groups"
	args := []string{"authorizer-test", "--authorizer.idp-endpoint", idp.server.URL, "--authorizer.rule", rule, "--server.address", "localhost:8080", "--client.endpoint", "localhost:8080", "--client.insecure-connection"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	jwt := idp.signToken("trusted-client", []string{"trusted-group"})

	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxgrpc.NewServerModule(conf),
		fxgrpc.NewClientModule(conf),
		health.Module,
		fxauthorizer.NewModule(conf),
		fx.Provide(
			// zap.NewDevelopment,
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
			healthpb.NewHealthClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
			fxgrpc.StartGrpcServer,
			func(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient, healthClient healthpb.HealthClient) {
				runGrpc(lc, sd, client, healthClient, jwt)
			},
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Endpoint returned status rpc error: code = PermissionDenied desc = policy denied
	// Healthcheck returned status:SERVING
}

func runGrpc(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient, healthClient healthpb.HealthClient, jwt string) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				defer sd.Shutdown() //nolint:errcheck
				authCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+jwt)
				_, err := client.GetFeature(authCtx, &pb.Point{})
				res, ok := status.FromError(err)
				if !ok {
					panic(fmt.Sprintln("could not extract grpc status code:", err))
				}
				fmt.Println("Endpoint returned status", res.String())

				resp, err := healthClient.Check(authCtx, &healthpb.HealthCheckRequest{})
				if err != nil {
					panic(fmt.Sprintln("healthcheck request returned error:", err))
				}
				fmt.Println("Healthcheck returned", resp.String())
			}()
			return nil
		},
	})
}

func Example_http() {
	idp := newTestIDP()
	defer idp.server.Close()

	conf := &HttpConfig{}
	rule := "request.path == '/health' || 'trusted-group' in request.jwt.groups"
	args := []string{"authorizer-test", "--authorizer.rule", rule, "--authorizer.idp-endpoint", idp.server.URL, "--server.address", "localhost:8081"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	jwt := idp.signToken("trusted-client", []string{"trusted-group"})

	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxhttp.NewModule(conf),
		fxauthorizer.NewModule(conf),
		fx.Provide(
			zap.NewNop,
			newMux,
		),
		fx.Invoke(
			fxhttp.StartHttpServer,
			func(lc fx.Lifecycle, sd fx.Shutdowner) {
				runHttp(lc, sd, jwt)
			},
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Request /foobar returned status 403 Forbidden
	// Request /health returned status 200 OK
	// Authenticated request /foobar returned status 404 Not Found
}

func newMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func runHttp(lc fx.Lifecycle, sd fx.Shutdowner, jwt string) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				defer sd.Shutdown() //nolint:errcheck
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8081/foobar", nil)
				if err != nil {
					panic(err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					panic(fmt.Sprintln("failed to do http request", err))
				}
				defer resp.Body.Close()
				fmt.Println("Request /foobar returned status", resp.Status)

				req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8081/health", nil)
				if err != nil {
					panic(err)
				}
				resp2, err := http.DefaultClient.Do(req2)
				if err != nil {
					panic(fmt.Sprintln("failed to do http request", err))
				}
				defer resp2.Body.Close()
				fmt.Println("Request /health returned status", resp2.Status)

				req3, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8081/foobar", nil)
				if err != nil {
					panic(err)
				}
				req3.Header.Set("Authorization", "Bearer "+jwt)
				resp3, err := http.DefaultClient.Do(req3)
				if err != nil {
					panic(fmt.Sprintln("failed to do http request", err))
				}
				defer resp3.Body.Close()
				fmt.Println("Authenticated request /foobar returned status", resp3.Status)
			}()
			return nil
		},
	})
}
