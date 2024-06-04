// Package reflection provides reflection capabilities for grpc servers.
package reflection

import (
	"go.uber.org/fx"
)

// Add a service that exposes the grpc server proto definition
// Deprecated: The reflection service is installed by default when using a grpc-server produced by the [fxgrpc] module
var Module = fx.Options()
