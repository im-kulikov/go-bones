package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

const defaultHTTPNetwork = "tcp"

func TestHTTPServer(t *testing.T) {
	t.Run("should fail on address already in use", func(t *testing.T) {
		lis, err := net.Listen(defaultHTTPNetwork, "127.0.0.1:0")
		require.NoError(t, err)

		defer func() { require.NoError(t, lis.Close()) }()

		serve := NewHTTPServer(WithHTTPLogger(logger.ForTests(t)), WithHTTPConfig(HTTPConfig{
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

	t.Run("stop", func(t *testing.T) {
		t.Run("should ignore empty server", func(t *testing.T) {
			serve := new(httpServer)
			require.NotPanics(t, func() { serve.Stop(context.Background()) })
		})

		t.Run("should not stop on canceled context", func(t *testing.T) {
			lis, errListen := net.Listen(defaultHTTPNetwork, "127.0.0.1:0")
			require.NoError(t, errListen)

			server := &http.Server{
				ReadHeaderTimeout: time.Second,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(time.Second)
				}),
			}

			serve := new(httpServer)
			serve.server = server
			serve.logger = logger.ForTests(t)

			wg := new(sync.WaitGroup)
			wg.Add(2) // client + server

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
			defer cancel()

			go func() {
				defer wg.Done()

				assert.EqualError(t, server.Serve(lis), http.ErrServerClosed.Error())
			}()

			go func() {
				defer wg.Done()

				// wait until start
				time.Sleep(time.Millisecond)

				cli := &http.Client{}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+lis.Addr().String(), nil)
				assert.NoError(t, err)

				// nolint:bodyclose
				_, err = cli.Do(req)
				assert.EqualError(t, errors.Unwrap(err), context.DeadlineExceeded.Error())
			}()

			time.Sleep(time.Millisecond * 10) // server should start
			require.NotPanics(t, func() {
				grace, stop := context.WithTimeout(ctx, time.Millisecond*10)
				defer stop()

				serve.Stop(grace)
			})

			wg.Wait() // wait until stop
		})
	})
}
