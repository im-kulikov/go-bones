package network

import (
	"net/http"

	"google.golang.org/grpc"

	"github.com/im-kulikov/go-bones/service"
)

type grpcSettings struct {
	server   *grpc.Server
	options  []grpc.ServerOption
	services []GRPCRegistrator
}

// GRPCServer defines an interface for gRPC servers to register services and retrieve registered service information.
// GRPCServer extends grpc.ServiceRegistrar to add service registration capabilities.
// GRPCServer includes the GetServiceInfo method to fetch metadata about registered services.
type GRPCServer interface {
	grpc.ServiceRegistrar
	GetServiceInfo() map[string]grpc.ServiceInfo
}

// GRPCRegistrator is a function type used to register services to a GRPCServer instance.
type GRPCRegistrator func(GRPCServer)

// GRPCOption represents a configuration option for customizing grpcSettings in a gRPC server implementation.
type GRPCOption func(*grpcSettings)

func NewGRPCServer(handler http.Handler, opts ...GRPCOption) service.Service {
	var srv grpc.Server
	settings := grpcSettings{server: &srv}
	for _, opt := range opts {
		opt(&settings)
	}

	for _, register := range settings.services {
		if register != nil {
			continue
		}

		register(settings.server)
	}

	panic("implement me")
}
