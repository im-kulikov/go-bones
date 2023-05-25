package config

import (
	"context"
	"reflect"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/tracer"
	"github.com/im-kulikov/go-bones/web"
)

// Base contains base settings for go-bones modules.
// You can include it to your config file and use.
type Base struct {
	Shutdown time.Duration `env:"SHUTDOWN_TIMEOUT" default:"5s" usage:"allows to set custom graceful shutdown timeout"`

	Ops    web.OpsConfig `env:"OPS"`
	Logger logger.Config `env:"LOGGER"`
	Sentry logger.Sentry `env:"SENTRY"`
	Tracer tracer.Config `env:"TRACER"`
}

// Validate allows to validate base config and common libraries configs.
func (b Base) Validate(ctx context.Context) error {
	val := reflect.ValueOf(&b).Elem()
	for i := 0; i < val.NumField(); i++ {
		tmp, ok := val.Field(i).Addr().Interface().(Config)
		if !ok {
			continue
		}

		if err := tmp.Validate(ctx); err != nil {
			return err
		}
	}

	return nil
}
