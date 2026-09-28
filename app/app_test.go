package app

import (
	"context"
	"errors"
	"testing"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/service"
)

type (
	exitCode int

	storeConfig struct{ Name string }
	apiConfig   struct{ Fail bool }

	settings struct {
		config.Base

		Store storeConfig
		API   apiConfig
	}

	store interface{ Lookup() string }
	repo  struct{ name string }
)

var errBroken = errors.New("broken")

func (r *repo) Lookup() string { return r.name }

func newRepo(cfg storeConfig, _ service.Env) (*repo, error) { return &repo{name: cfg.Name}, nil }

// newAPI is a service that needs the store and stops right away, so Run returns.
func newAPI(cfg apiConfig, env service.Env) (service.Service, error) {
	s := service.Get[store](env)

	return service.NewLauncher("api", func(context.Context) error {
		if cfg.Fail {
			return errBroken
		}

		if s.Lookup() == "" {
			return errors.New("empty store")
		}

		return nil
	}), nil
}

func newBroken(apiConfig, service.Env) (service.Service, error) { return nil, errBroken }

// setup isolates the process-wide application and turns exit into a panic.
func setup(t *testing.T) {
	t.Helper()

	t.Setenv("LOGGER_LEVEL", "error")
	t.Setenv("OPS_ADDRESS", "127.0.0.1:0")

	prevStd, prevExit := std, exit
	exit = func(code int) { panic(exitCode(code)) }

	t.Cleanup(func() {
		if std != nil && std.cancel != nil {
			std.cancel()
		}

		std, exit = prevStd, prevExit
	})

	std = nil
}

func initApp() *settings {
	return Init[settings](
		config.WithName("app-test"),
		config.WithCustomizeLoaderConfig(func(c *gonfig.Config) { c.Args = []string{} }))
}

func exitOf(t *testing.T, fn func()) (code int) {
	t.Helper()

	code = -1
	defer func() {
		if r := recover(); r != nil {
			c, ok := r.(exitCode)
			require.True(t, ok, "unexpected panic: %v", r)
			code = int(c)
		}
	}()

	fn()

	return code
}

func TestApp_InitAddRun(t *testing.T) {
	setup(t)

	cfg := initApp()
	cfg.Store.Name = "db"

	r := Add(cfg.Store, newRepo)
	require.Equal(t, "db", r.name, "Add returns what it built")

	Add(cfg.API, newAPI)
	require.Len(t, std.list, 1, "the repo is not a service")
	require.NotNil(t, std.ops)

	require.Equal(t, -1, exitOf(t, Run), "a clean shutdown does not exit")
}

func TestApp_ServiceFailureExits(t *testing.T) {
	setup(t)

	cfg := initApp()
	Add(storeConfig{Name: "db"}, newRepo)
	cfg.API.Fail = true
	Add(cfg.API, newAPI)

	require.Equal(t, 1, exitOf(t, Run))
}

func TestApp_AddFailures(t *testing.T) {
	setup(t)

	cfg := initApp()

	require.Equal(t, 1, exitOf(t, func() { Add(cfg.API, newAPI) }), "missing dependency")
	require.Equal(t, 1, exitOf(t, func() { Add(cfg.API, newBroken) }), "constructor error")
	require.Empty(t, std.list)
}

func TestApp_InitFailures(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		setup(t)
		t.Setenv("HEALTH_INTERVAL", "not a duration")

		require.Equal(t, 1, exitOf(t, func() { initApp() }))
		require.Nil(t, std)
	})

	t.Run("ops routes", func(t *testing.T) {
		setup(t)
		t.Setenv("OPS_LIVE_PATH", "/")
		t.Setenv("OPS_READY_PATH", "/")

		require.Equal(t, 1, exitOf(t, func() { initApp() }))
		require.Nil(t, std)
	})
}

func TestApp_RequiresInit(t *testing.T) {
	setup(t)

	require.PanicsWithValue(t, "app: Init must be called before Add and Run", Run)
}

// TestApp_ExitThatReturns covers the paths after exit for an exit function that
// returns (os.Exit never does): Init and Add still return without a half-built app.
func TestApp_ExitThatReturns(t *testing.T) {
	setup(t)

	var codes []int
	exit = func(code int) { codes = append(codes, code) }

	t.Setenv("HEALTH_INTERVAL", "not a duration")
	require.NotNil(t, initApp())
	require.Nil(t, std)

	t.Setenv("HEALTH_INTERVAL", "10s")
	t.Setenv("OPS_LIVE_PATH", "/")
	t.Setenv("OPS_READY_PATH", "/")
	require.NotNil(t, initApp())
	require.Nil(t, std)

	t.Setenv("OPS_READY_PATH", "/readyz")
	cfg := initApp()
	require.Nil(t, Add(cfg.API, newBroken))

	require.Equal(t, []int{1, 1, 1}, codes)
}
