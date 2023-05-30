package grpc_example

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type testGRPCService struct {
	name string

	UnimplementedExampleGRPCServiceServer
}

var _ ExampleGRPCServiceServer = (*testGRPCService)(nil)

// Ping emulate useful service.
func (*testGRPCService) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	if tid := trace.SpanFromContext(ctx).SpanContext().TraceID(); tid.IsValid() {
		return &PingResponse{Message: tid.String()}, nil
	}

	return &PingResponse{Message: req.Name}, nil
}

// Name of the service.
func (t *testGRPCService) Name() string { return t.name }

// Register service on grpc.Server.
func (t *testGRPCService) Register(srv *grpc.Server) {
	RegisterExampleGRPCServiceServer(srv, t)
}

// NewTestService it's example package, so we should not care about exports
//
//goland:noinspection GoExportedFuncWithUnexportedType
func NewTestService(name ...string) *testGRPCService {
	return &testGRPCService{name: strings.Join(name, "-")}
}

// Register service on grpc.Server.
func Register(server *grpc.Server) {
	NewTestService().Register(server)
}
