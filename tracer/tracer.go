package tracer

import (
	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// Config provides configuration for jaeger tracer.
type Config struct {
	Type    Type `env:"TYPE" default:"jaeger" usage:"allows to set trace exporter type"`
	Enabled bool `env:"ENABLED" default:"false" usage:"allows to enable tracing"`

	Jaeger
}

// Type of tracer component.
type Type string

// JaegerType allows use jaeger as tracer.
const JaegerType = Type("jaeger")

var errUnknownType = bones.Error{
	Code:    bones.ErrorCodeInternal,
	Message: "unknown tracer type",
}

// Init configure tracer component and prepares it to work.
func Init(log logger.Logger, cfg Config, opts ...Option) (service.Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	switch cfg.Type {
	case JaegerType:
		return prepareJaeger(log, cfg.Jaeger, opts...)
	default:
		return nil, errUnknownType
	}
}
