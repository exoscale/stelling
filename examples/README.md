# Exoscale Go project skeleton

This module contains the standard skeleton for Stelling-based Golang projects.

You should more or less be able to copy-paste this directory and the associated Github Actions into a new repo.

Note that you will need to add a "push" step for the image builds to push to your own container repo.

The example contains:

- Config loading
- An OpenAPI spec with code generation
- HTTP and gRPC servers
- A job (e.g. to run as a scheduled task)
- An Earthfile, which can be replaced by a Dockerfile/Makefile for *simple* projects
- Github Actions (found in the top-level `.github/workflows` directory, prefixed with `example-`)
- Renovate (found in the top-level `renovate.json` file)

The code is heavily commented and should be straightforward to modify.

Detailed information on the capabilities of each module, can be found in the module README and example tests

## Usage

### HTTP server

```
# Start the server
go run ./cmd/http-server

# Make a request to the public api
curl http://localhost:8080/stelling/greeting

# Make a request to the internal api
curl http://localhost:8080/internal/healthz
```

### gRPC server

```
# Start the server
go run ./cmd/grpc-server

# Make a request
grpcurl -plaintext localhost:8080 exoscale.examples.v1.Greeter/Greeting
```

### Protobuf + gRPC tooling

If you're working with Protobuf/gRPC, it's useful to install the tooling, e.g.

```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

See the [getting started guide](https://protobuf.dev/getting-started/gotutorial/) for more details.

You will also need to install [buf](https://buf.build/docs/cli/installation/).

### Configuration

You can override config with environment variables, e.g.

```
export CONFIG_GREETING_MESSAGE="this is a custom message"
go run ./cmd/http-server

curl http://localhost:8080/stelling/greeting
```

### Docker compose

```
docker compose up http-server
curl http://localhost:8080/stelling/greeting

docker compose up grpc-server
grpcurl -plaintext localhost:8081 exoscale.examples.v1.Greeter/Greeting
```
