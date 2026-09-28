// Package app is an optional facade that assembles a go-bones application from
// component constructors, with production defaults.
//
//	func main() {
//		cfg := app.Init[settings](config.WithVersion(version))
//
//		app.Add(cfg.Storage, storage.New)
//		app.Add(cfg.API, api.New) // api.New gets the storage with service.Get
//
//		app.Run()
//	}
//
// Init loads the configuration and sets up the logger, OpenTelemetry, the health
// monitor and the OPS server with health probes. Add builds components in order;
// each constructor gets the application service.Env and finds what was built
// before it with service.Get. Run starts every service, waits for a signal or a
// failure and shuts down in phases: readiness off, drain delay, services, then
// the OPS server and the monitor.
//
// There is one application per process, like flag.CommandLine. Components stay
// independent of this package: they only use service.Env, and are tested with
// service.TestEnv.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"
	"github.com/im-kulikov/go-bones/tracer"
)

type application struct {
	top    context.Context
	cancel context.CancelFunc
	base   *config.Base
	env    service.Env
	health *health.Monitor
	ops    service.Service
	list   []service.Service
}

var (
	// std is the process-wide application, like flag.CommandLine.
	std *application //nolint:gochecknoglobals

	// exit is os.Exit, replaced in tests.
	exit = os.Exit //nolint:gochecknoglobals
)

// Init loads the application configuration into a new C and prepares the
// application: logger, OpenTelemetry, health monitor, OPS server and a context
// canceled on SIGINT/SIGTERM. C must embed config.Base; forgetting it is a
// compile error. A configuration or OPS setup error exits with code 1.
func Init[C any, PC interface {
	*C
	Bones() *config.Base
}](options ...config.Option) *C {
	cfg := new(C)
	if err := config.Load(cfg, options...); err != nil {
		fail(logger.Default(), "could not load config", err)

		return cfg
	}

	base := PC(cfg).Bones()
	log := logger.Init(base.Logger)
	monitor := health.New(base.Health, log)

	ops, err := http.NewOPSServer(base.OpsServer, log, http.WithHealth(monitor))
	if err != nil {
		fail(log, "could not create OPS server", err)

		return cfg
	}

	ctx, cancel := service.SignalContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	std = &application{
		top:    context.Background(),
		cancel: cancel,
		base:   base,
		env:    service.NewEnv(ctx, log, monitor),
		health: monitor,
		ops:    ops,
	}

	std.add(tracer.Init(log, base.Tracer))

	return cfg
}

// Add builds a component with ctor from cfg and returns it. The constructor gets
// the application service.Env and can service.Get anything added before it.
// A service is started by Run. A constructor error or an unresolved dependency
// is logged with the file and line of the Add call and exits with code 1.
func Add[C, T any](cfg C, ctor service.Constructor[C, T]) T {
	a := current()

	v, err := service.Build(a.env, cfg, ctor)
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fail(
			a.env.Logger,
			fmt.Sprintf("%s:%d: could not build component", filepath.Base(file), line),
			err,
		)

		return v
	}

	if svc, ok := any(v).(service.Service); ok {
		a.add(svc)
	}

	return v
}

// Run starts every added service and the OPS server, and blocks until a signal
// or a service failure, then shuts down in phases (see service.WithHealth and
// service.WithDrainDelay). A clean shutdown returns; a failure exits with code 1.
func Run() {
	a := current()
	defer a.cancel()

	err := service.RunContext(a.top, a.env.Logger,
		service.WithHealth(a.health),
		service.WithDrainDelay(a.base.Health.DrainDelay),
		service.WithShutdownLast(a.ops),
		service.WithService(a.list...),
		service.WithService(a.ops))
	if err != nil {
		fail(a.env.Logger, "application stopped with error", err)
	}
}

func (a *application) add(svc service.Service) {
	if svc != nil {
		a.list = append(a.list, svc)
	}
}

func current() *application {
	if std == nil {
		panic("app: Init must be called before Add and Run")
	}

	return std
}

func fail(log *logger.Logger, msg string, err error) {
	log.Error(msg, logger.Err(err))
	exit(1)
}
