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
	reflection "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
	example "github.com/im-kulikov/go-bones/web/grpc_example"
)

const testGRPCServiceName = "grpc-test"

func TestNewGRPCServer(t *testing.T) {
	lis, errListen := net.Listen(defaultGRPCNetwork, "127.0.0.1:0")
	require.NoError(t, errListen)
	require.NoError(t, lis.Close())

	log := logger.ForTests(t)
	svc := example.NewTestService(testGRPCServiceName)

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
		WithGRPCService(svc),
		// pass custom gRPC config
		WithGRPCConfig(GRPCConfig{
			Address: lis.Addr().String(),
			Network: lis.Addr().Network(),
			Enabled: true,
			Reflect: true,
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

		{ // example ping
			var res *example.PingResponse
			res, err = example.NewExampleGRPCServiceClient(conn).Ping(context.Background(),
				&example.PingRequest{Name: testGRPCServiceName})
			assert.NoError(t, err)

			assert.Equal(t, testGRPCServiceName, res.Message)
		}

		{ // call reflection service
			var cli reflection.ServerReflection_ServerReflectionInfoClient
			cli, err = reflection.NewServerReflectionClient(conn).ServerReflectionInfo(context.TODO())
			assert.NoError(t, err)

			// should be fixed when it would be changed in reflection service.
			assert.NoError(t, cli.Send(&reflection.ServerReflectionRequest{ // nolint:staticcheck
				Host:           "*",
				MessageRequest: &reflection.ServerReflectionRequest_ListServices{},
			}))

			// should be fixed when it would be changed in reflection service.
			assert.NoError(t, cli.CloseSend())

			var res *reflection.ServerReflectionResponse // nolint:staticcheck
			res, err = cli.Recv()
			assert.NoError(t, err)

			list, ok := res.MessageResponse.(*reflection.ServerReflectionResponse_ListServicesResponse)
			assert.True(t, ok)

			expect := []string{"ExampleGRPCService", "grpc.reflection.v1alpha.ServerReflection"}
			actual := make([]string, 0, len(list.ListServicesResponse.Service)) // nolint:staticcheck
			for _, item := range list.ListServicesResponse.Service {            // nolint:staticcheck
				actual = append(actual, item.Name) // nolint:staticcheck
			}

			assert.Equal(t, expect, actual)
		}
	}()

	require.NoError(t, serve.Start(context.Background()))
}

func TestGRPCServer(t *testing.T) {
	t.Run("should fail on address already in use", func(t *testing.T) {
		lis, err := net.Listen(defaultGRPCNetwork, "127.0.0.1:0")
		require.NoError(t, err)

		defer func() { require.NoError(t, lis.Close()) }()

		serve := NewGRPCServer(WithGRPCLogger(logger.ForTests(t)), WithGRPCConfig(GRPCConfig{
			Enabled: false,
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
