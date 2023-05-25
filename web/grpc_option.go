package web

import (
	"google.golang.org/grpc"

	"github.com/im-kulikov/go-bones/logger"
)

// GRPCOption allows customizing gRPC server.
type GRPCOption func(s *gRPCServer)

// GRPCService custom interface for gRPC service.
type GRPCService interface {
	Name() string
	Register(server *grpc.Server)
}

// WithGRPCName allows set custom gRPC Name.
func WithGRPCName(v string) GRPCOption {
	return func(s *gRPCServer) { s.name = v }
}

// WithGRPCConfig allows set custom gRPC settings.
func WithGRPCConfig(v GRPCConfig) GRPCOption {
	return func(s *gRPCServer) { s.GRPCConfig = v }
}

// WithGRPCLogger allows set custom gRPC Logger.
func WithGRPCLogger(v logger.Logger) GRPCOption {
	return func(s *gRPCServer) { s.logger = v }
}

// WithGRPCServer allows set custom gRPC Server.
func WithGRPCServer(v *grpc.Server) GRPCOption {
	return func(s *gRPCServer) { s.server = v }
}

// WithGRPCService allows adding new gRPC Service.
func WithGRPCService(v GRPCService) GRPCOption {
	return func(s *gRPCServer) { s.services = append(s.services, v) }
}
