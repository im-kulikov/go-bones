package web

import (
	"context"
	"net"

	gprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// GRPCConfig provides configuration for http server.
type GRPCConfig struct {
	Enabled bool   `env:"ENABLED" default:"false" usage:"allows to enable grpc server"`
	Reflect bool   `env:"REFLECT" default:"false" usage:"allows to enable grpc reflection service"`
	Address string `env:"ADDRESS" default:":9080" usage:"gRPC server listen address"`
	Network string `env:"NETWORK" default:"tcp" usage:"gRPC server listen network: tpc/udp"`
}

type gRPCServer struct {
	GRPCConfig

	name     string
	server   *grpc.Server
	logger   logger.Logger
	services []GRPCService
}

func defaultGRPCServer() *grpc.Server {
	return grpc.NewServer(
		grpc.ChainUnaryInterceptor(gprom.UnaryServerInterceptor, otelgrpc.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(gprom.StreamServerInterceptor, otelgrpc.StreamServerInterceptor()),
	)
}

const (
	defaultGRPCName    = "grpc-server"
	defaultGRPCAddress = ":9080"
	defaultGRPCNetwork = "tcp"
)

// NewGRPCServer creates new gRPC server and implements service.Service interface.
func NewGRPCServer(opts ...GRPCOption) service.Service {
	serve := &gRPCServer{
		name:   defaultGRPCName,
		logger: logger.Default(),
		server: defaultGRPCServer(),

		GRPCConfig: GRPCConfig{
			Enabled: true,
			Address: defaultGRPCAddress,
			Network: defaultGRPCNetwork,
		},
	}

	for _, o := range opts {
		o(serve)
	}

	if serve.Reflect {
		serve.services = append(serve.services, new(reflectionService))
	}

	for i := range serve.services {
		if serve.services[i] == nil {
			serve.logger.Warnf("empty gRPC service #%d", i)

			continue
		}

		serve.logger.Infow("register gRPC service", "service", serve.services[i].Name())

		serve.services[i].Register(serve.server)
	}

	return serve
}

// Name returns name of gRPC server.
func (s *gRPCServer) Name() string { return s.name }

// Enabled returns is service enabled.
func (s *gRPCServer) Enabled() bool { return s.GRPCConfig.Enabled }

// Start allows starting gRPC server.
func (s *gRPCServer) Start(ctx context.Context) error {
	s.logger.Infow("prepare listener",
		"name", s.name,
		"address", s.Address,
		"network", s.Network)

	lis, err := new(net.ListenConfig).Listen(ctx, s.Network, s.Address)
	if err != nil {
		return err
	}

	return s.server.Serve(lis)
}

// Stop allows to stop http server.
func (s *gRPCServer) Stop(context.Context) {
	if s.server == nil {
		return
	}

	s.server.GracefulStop()
}
