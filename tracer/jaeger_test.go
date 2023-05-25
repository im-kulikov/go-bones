package tracer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/examples/helloworld/helloworld"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
	"github.com/im-kulikov/go-bones/web"
)

type testGRPCService struct {
	*testing.T
	helloworld.GreeterServer
}

type catchBufferSync struct {
	mock.Mock

	callback func(data []byte)
}

const testEndpoint = "http://jaeger:62254"

var (
	_ logger.WriteSyncer       = (*catchBufferSync)(nil)
	_ helloworld.GreeterServer = (*testGRPCService)(nil)
)

func (t *testGRPCService) SayHello(ctx context.Context, req *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	if tid := trace.SpanFromContext(ctx).SpanContext().TraceID().String(); tid != "" {
		return &helloworld.HelloReply{Message: tid}, nil
	}

	return &helloworld.HelloReply{Message: req.Name}, nil
}

func (c *catchBufferSync) Write(data []byte) (int, error) {
	args := c.Called(data)

	if c.callback != nil {
		c.callback(data)
	}

	return len(data), args.Error(0)
}

func (c *catchBufferSync) Sync() error { return nil }

func (c *catchBufferSync) Close() error { return nil }

func prepareConfig(t *testing.T, envs ...string) Config {
	var cfg Config

	t.Helper()

	require.NoError(t, aconfig.LoaderFor(&cfg, aconfig.Config{
		Args: []string{},
		Envs: envs,
	}).Load())

	return cfg
}

const (
	testNetwork = "tcp"
	testAddress = "127.0.0.1:0"
)

func prepareTestAddress(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen(testNetwork, testAddress)
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	return lis.Addr().String()
}

func newTestGRPCService(t *testing.T, log logger.Logger) (service.Service, string) {
	svc := new(testGRPCService)
	svc.T = t

	serve := grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()))
	helloworld.RegisterGreeterServer(serve, svc)

	address := prepareTestAddress(t)

	return web.NewGRPCServer(
			web.WithGRPCLogger(log),
			web.WithGRPCServer(serve),
			web.WithGRPCName("test-grpc"),
			web.WithGRPCConfig(web.GRPCConfig{Address: address, Network: testNetwork})),
		address
}

func newTestHTTPService(t *testing.T, log logger.Logger) (service.Service, string) {
	address := prepareTestAddress(t)

	return web.NewHTTPServer(
			web.WithHTTPLogger(log),
			web.WithHTTPName("test-http"),
			web.WithHTTPHandler(http.NotFoundHandler()),
			web.WithHTTPConfig(web.HTTPConfig{Address: address, Network: testNetwork})),
		address
}

type assertion func(t *testing.T)

func newHTTPCase(ctx context.Context, hAddress string) assertion {
	return func(t *testing.T) {
		cli := new(http.Client)
		web.ApplyTracingToHTTPClient(cli)

		uri := (&url.URL{Scheme: "http", Host: hAddress}).String()

		req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if !assert.NoError(t, errReq) {
			return
		}

		res, errRes := cli.Do(req)
		assert.NoError(t, errRes, hAddress)
		if assert.NotNil(t, res) {
			assert.NoError(t, res.Body.Close())
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		}
	}
}

type grpcAssertion func(ctx context.Context, t *testing.T, cli helloworld.GreeterClient)

func newGRPCCase(ctx context.Context, gAddress string, check grpcAssertion) assertion {
	return func(t *testing.T) {
		t.Helper()

		con, errDial := grpc.DialContext(ctx, gAddress,
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()))

		if !assert.NoError(t, errDial, gAddress) {
			return
		}

		check(ctx, t, helloworld.NewGreeterClient(con))
	}
}

func testCases(top context.Context, t *testing.T, hAddress, gAddress string) {
	t.Helper()

	cases := []struct {
		name string
		call assertion
	}{
		{
			name: "http client without request-id",
			call: newHTTPCase(top, hAddress),
		},
		{
			name: "gRPC client propagate from context",
			call: newGRPCCase(top, gAddress, func(ctx context.Context, t *testing.T, cli helloworld.GreeterClient) {
				t.Helper()

				stx, span := otel.Tracer("from-test").Start(ctx, "span-name")
				defer span.End()

				res, errRes := cli.SayHello(stx, &helloworld.HelloRequest{Name: testEndpoint})
				assert.NoError(t, errRes)
				if assert.NotNil(t, res) {
					assert.Equal(t, span.SpanContext().TraceID().String(), res.Message)
				}
			}),
		},
		{
			name: "gRPC client propagate from request headers",
			call: newGRPCCase(top, gAddress, func(ctx context.Context, t *testing.T, cli helloworld.GreeterClient) {
				t.Helper()

				var tid trace.TraceID

				stx, span := otel.Tracer("from-test").Start(ctx, "came from test")
				defer span.End()

				tid = span.SpanContext().TraceID()

				srv := httptest.NewServer(web.HTTPTracingMiddlewareFunc(func(_ http.ResponseWriter, r *http.Request) {
					spanHandler := trace.SpanFromContext(r.Context())
					defer spanHandler.End()

					assert.Equal(t, tid, spanHandler.SpanContext().TraceID(), "incoming from http request")

					res, errRes := cli.SayHello(r.Context(), &helloworld.HelloRequest{Name: testEndpoint})
					assert.NoError(t, errRes)
					if assert.NotNil(t, res) {
						assert.Equal(t, tid.String(), res.Message, "incoming from grpc request")
					}
				}))

				req, err := http.NewRequestWithContext(stx, http.MethodGet, srv.URL, nil)
				if !assert.NoError(t, err) {
					return
				}

				httpClient := srv.Client()
				web.ApplyTracingToHTTPClient(httpClient)

				res, err := httpClient.Do(req)
				if !assert.NoError(t, err) {
					return
				}

				assert.NoError(t, res.Body.Close())
			}),
		},

		{
			name: "gRPC client propagate from uber-trace-id headers",
			call: newGRPCCase(top, gAddress, func(ctx context.Context, t *testing.T, cli helloworld.GreeterClient) {
				t.Helper()

				var tid trace.TraceID

				data, err := hex.DecodeString("5e5bd842bb952b7b7a4bc19e1207f4b0")
				if !assert.NoError(t, err) {
					return
				}

				copy(tid[:], data)

				srv := httptest.NewServer(web.HTTPTracingMiddlewareFunc(func(_ http.ResponseWriter, r *http.Request) {
					span := trace.SpanFromContext(r.Context())
					defer span.End()

					assert.Equal(t, tid, span.SpanContext().TraceID(), "incoming from http request")

					res, errRes := cli.SayHello(r.Context(), &helloworld.HelloRequest{Name: testEndpoint})
					assert.NoError(t, errRes)
					if assert.NotNil(t, res) {
						assert.Equal(t, tid.String(), res.Message, "incoming from grpc request")
					}
				}))

				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
				if !assert.NoError(t, err) {
					return
				}

				req.Header.Add("uber-trace-id", "5e5bd842bb952b7b7a4bc19e1207f4b0:22d397bd97eb30a7:0:1")

				res, err := srv.Client().Do(req)
				if !assert.NoError(t, err) {
					return
				}

				assert.NoError(t, res.Body.Close())
			}),
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(cases))

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			defer wg.Done()

			tt.call(t)
		})
	}

	wg.Wait()
}

type Flusher interface {
	Flush(context.Context) error
}

type fakeSink struct {
	io.Writer
}

func (fakeSink) Sync() error { return nil }

func (fakeSink) Close() error { return nil }

const errLookupHost = "UDP connection not yet initialized, an address has not been resolved"

func TestNew_Jaeger(t *testing.T) {
	t.Run("should be ok", func(t *testing.T) {
		cfg := prepareConfig(t,
			"AGENT_HOST=jaeger",
			"AGENT_PORT=62254",
		)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		tracer, err := Init(logger.ForTests(t), cfg,
			WithJaegerServiceName("test-service"),
			WithJaegerServiceVersion("test-version"),
			WithJaegerServiceEnv("test"),
			WithCustomErrorHandler(func(err error) {
				assert.Error(t, err)

				assert.Contains(t, err.Error(), errLookupHost)
			}))

		require.NoError(t, err)

		gServe, gAddress := newTestGRPCService(t, logger.ForTests(t))
		hServe, hAddress := newTestHTTPService(t, logger.ForTests(t))

		go func() {
			time.Sleep(time.Millisecond * 10)

			// run sub tests...
			testCases(ctx, t, hAddress, gAddress)
		}()

		require.NoError(t, service.New(logger.ForTests(t),
			service.WithService(tracer),
			service.WithService(gServe),
			service.WithService(hServe),
			service.WithShutdownTimeout(time.Millisecond)).Run(ctx))

		if tmp, ok := tracer.(Flusher); ok {
			require.NoError(t, tmp.Flush(context.Background()))
		}
	})

	t.Run("should fail on dial", func(t *testing.T) {
		cfg := prepareConfig(t, "ENDPOINT=http://jaeger:64444")
		buf := new(bytes.Buffer)

		log, err := logger.New(logger.Config{}, logger.WithCustomOutput("test", fakeSink{Writer: buf}))
		require.NoError(t, err)

		svc, err := Init(log, cfg)
		require.NoError(t, err)

		srv := httptest.NewServer(web.HTTPTracingMiddleware(http.NotFoundHandler()))
		cli := srv.Client()
		web.ApplyTracingToHTTPClient(cli)

		res, err := cli.Get(srv.URL)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		err = svc.(Flusher).Flush(context.Background())
		require.Error(t, errors.Unwrap(err))
		require.Contains(t, errors.Unwrap(err).Error(), "dial tcp: lookup jaeger")

		time.Sleep(time.Millisecond * 100)

		grace, done := context.WithCancel(context.Background())
		done()

		require.NotPanics(t, func() { svc.Stop(grace) })
		require.Contains(t, buf.String(), "could not shutdown export provider")
	})

	t.Run("should call default error handler", func(t *testing.T) {
		cfg := prepareConfig(t,
			"AGENT_HOST=jaeger",
			"AGENT_PORT=5555")

		buf := new(bytes.Buffer)
		log, err := logger.New(logger.Config{},
			logger.WithCustomOutput("error-handler", &fakeSink{Writer: buf}))
		require.NoError(t, err)

		_, err = Init(log, cfg)
		require.NoError(t, err)

		otel.Handle(err)                                // should ignore error
		otel.Handle(errors.New(errCantUploadTraceSpan)) // should write to logger

		require.Contains(t, buf.String(), errCantUploadTraceSpan)

		t.Log(buf.String())
	})

	t.Run("should fail on invalid agent", func(t *testing.T) {
		cfg := prepareConfig(t,
			"AGENT_HOST=host",
			"AGENT_PORT=",
			"AGENT_RETRY_COUNT=0",
			"AGENT_RETRY_INTERVAL=-1s")

		_, err := Init(logger.ForTests(t), cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "lookup host")
	})

	t.Run("should fail on unknown export", func(t *testing.T) {
		cfg := prepareConfig(t)
		err := testInit(logger.ForTests(t), cfg)
		require.EqualError(t, err, errUnknownType.Error(), "%#+v", err)
	})

	t.Run("should do nothing when disabled", func(t *testing.T) {
		require.NoError(t, testInit(logger.ForTests(t), Config{Disable: true}))
	})

	t.Run("should fail on unknown type", func(t *testing.T) {
		require.EqualError(t, testInit(logger.ForTests(t), Config{Type: "unknown-type"}), errUnknownType.Error())
	})
}

func testInit(log logger.Logger, cfg Config) error {
	_, err := Init(log, cfg)

	return err
}
