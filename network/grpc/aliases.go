package grpc

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Server aliases google.golang.org/grpc.Server for service registration callbacks.
type Server = grpc.Server

// ServerOption aliases google.golang.org/grpc.ServerOption for server construction helpers.
type ServerOption = grpc.ServerOption

// ServiceRegistrar aliases google.golang.org/grpc.ServiceRegistrar for generated registration helpers.
type ServiceRegistrar = grpc.ServiceRegistrar

// ServiceDesc aliases google.golang.org/grpc.ServiceDesc for custom service descriptors.
type ServiceDesc = grpc.ServiceDesc

// MethodDesc aliases google.golang.org/grpc.MethodDesc for custom service descriptors.
type MethodDesc = grpc.MethodDesc

// StreamDesc aliases google.golang.org/grpc.StreamDesc for custom service descriptors.
type StreamDesc = grpc.StreamDesc

// ServerStream aliases google.golang.org/grpc.ServerStream for stream handlers.
type ServerStream = grpc.ServerStream

// ClientConn aliases google.golang.org/grpc.ClientConn for client helpers.
type ClientConn = grpc.ClientConn

// DialOption aliases google.golang.org/grpc.DialOption for client helpers.
type DialOption = grpc.DialOption

// UnaryHandler aliases google.golang.org/grpc.UnaryHandler for custom descriptors.
type UnaryHandler = grpc.UnaryHandler

// StreamHandler aliases google.golang.org/grpc.StreamHandler for custom descriptors.
type StreamHandler = grpc.StreamHandler

// UnaryServerInfo aliases google.golang.org/grpc.UnaryServerInfo for custom descriptors.
type UnaryServerInfo = grpc.UnaryServerInfo

// StreamServerInfo aliases google.golang.org/grpc.StreamServerInfo for custom descriptors.
type StreamServerInfo = grpc.StreamServerInfo

// UnaryServerInterceptor aliases google.golang.org/grpc.UnaryServerInterceptor.
type UnaryServerInterceptor = grpc.UnaryServerInterceptor

// StreamServerInterceptor aliases google.golang.org/grpc.StreamServerInterceptor.
type StreamServerInterceptor = grpc.StreamServerInterceptor

// nolint:gochecknoglobals
var (
	// ErrServerStopped re-exports google.golang.org/grpc.ErrServerStopped for lifecycle checks.
	ErrServerStopped = grpc.ErrServerStopped

	// NewClient aliases google.golang.org/grpc.NewClient for tests and client helpers.
	NewClient = grpc.NewClient

	// MaxRecvMsgSize aliases google.golang.org/grpc.MaxRecvMsgSize for server options.
	MaxRecvMsgSize = grpc.MaxRecvMsgSize

	// WithTransportCredentials aliases google.golang.org/grpc.WithTransportCredentials for client helpers.
	WithTransportCredentials = grpc.WithTransportCredentials
)

func chainUnaryInterceptor(interceptors ...UnaryServerInterceptor) ServerOption {
	return grpc.ChainUnaryInterceptor(interceptors...)
}

func chainStreamInterceptor(interceptors ...StreamServerInterceptor) ServerOption {
	return grpc.ChainStreamInterceptor(interceptors...)
}

func withTransportCredentials(creds credentials.TransportCredentials) ServerOption {
	return grpc.Creds(creds)
}

func newServer(opts ...ServerOption) *Server {
	return grpc.NewServer(opts...)
}
