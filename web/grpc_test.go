package web

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/examples/helloworld/helloworld"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type testGRPCService struct {
	helloworld.GreeterServer
}

const testGRPCServiceName = "grpc-test"

var _ helloworld.GreeterServer = (*testGRPCService)(nil)

func (t *testGRPCService) SayHello(_ context.Context, req *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	return &helloworld.HelloReply{Message: req.Name}, nil
}

func (t *testGRPCService) Name() string {
	return helloworld.Greeter_ServiceDesc.ServiceName
}

func (t *testGRPCService) Register(server *grpc.Server) {
	helloworld.RegisterGreeterServer(server, t)
}

func newTestService() GRPCService {
	return new(testGRPCService)
}

func TestNewGRPCServer(t *testing.T) {
	lis, errListen := net.Listen(defaultGRPCNetwork, "127.0.0.1:0")
	require.NoError(t, errListen)
	require.NoError(t, lis.Close())

	log := logger.ForTests(t)

	hello := newTestService()

	server := grpc.NewServer()

	serve := NewGRPCServer(
		// pass custom gRPC server
		WithGRPCServer(server),
		// pass custom gRPC logger
		WithGRPCLogger(log),
		// pass custom gRPC server name
		WithGRPCName(testGRPCServiceName),
		// pass custom gRPC service
		WithGRPCService(nil), // should be ignored
		WithGRPCService(hello),
		// pass custom gRPC config
		WithGRPCConfig(GRPCConfig{
			Address: lis.Addr().String(),
			Network: lis.Addr().Network(),
			Disable: false,
		}))

	require.Equal(t, testGRPCServiceName, serve.Name())
	require.Implements(t, (*service.Enabler)(nil), serve)
	require.True(t, serve.(service.Enabler).Enabled())

	go func() {
		defer serve.Stop(context.Background())

		conn, err := grpc.Dial(lis.Addr().String(),
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		assert.NoError(t, err)

		res, err := helloworld.NewGreeterClient(conn).SayHello(context.Background(),
			&helloworld.HelloRequest{Name: testGRPCServiceName})
		assert.NoError(t, err)

		assert.Equal(t, testGRPCServiceName, res.Message)
	}()

	require.NoError(t, serve.Start(context.Background()))
}

func TestGRPCServer(t *testing.T) {
	t.Run("should fail on address already in use", func(t *testing.T) {
		lis, err := net.Listen(defaultGRPCNetwork, "127.0.0.1:0")
		require.NoError(t, err)

		defer func() { require.NoError(t, lis.Close()) }()

		serve := NewGRPCServer(WithGRPCLogger(logger.ForTests(t)), WithGRPCConfig(GRPCConfig{
			Disable: true,
			Address: lis.Addr().String(),
			Network: lis.Addr().Network(),
		}))

		err = serve.Start(context.Background())
		require.EqualError(t, errors.Unwrap(err), (&os.SyscallError{
			Syscall: "bind",
			Err:     syscall.EADDRINUSE,
		}).Error())
	})

	t.Run("stop should ignore empty server", func(t *testing.T) {
		serve := new(gRPCServer)
		serve.logger = logger.ForTests(t)
		require.NotPanics(t, func() { serve.Stop(context.Background()) })
	})
}
