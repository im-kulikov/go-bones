package config

import (
	"crypto/tls"
	"fmt"
	"reflect"
	"time"

	"github.com/im-kulikov/go-bones"
)

// Base provides the standard configuration structure for go-bones applications.
// It groups logger, operations server, and tracer configuration under a single root.
type Base struct {
	Logger    Logger       `env:"LOGGER" yaml:"logger" json:"logger" toml:"logger"`
	OpsServer Ops          `env:"OPS"    yaml:"ops"    json:"ops"    toml:"ops"`
	Tracer    TracerConfig `env:"OTEL"   yaml:"tracer" json:"tracer" toml:"tracer"`
}

type appSettings struct {
	name    string
	version string
}

type appSetter interface {
	SetAppNameAndVersion(name, version string)
}

// Network defines the foundational configuration for network-based services,
// including timeouts and TLS settings.
//
//nolint:lll,golines // Struct tags keep all supported config backends visible per field.
type Network struct {
	appSettings

	TLSConfig         *TLS          `toml:"tls" yaml:"tls" json:"tls" env:"TLS"`
	ReadTimeout       time.Duration `toml:"read_timeout" yaml:"read_timeout" json:"readTimeout" env:"READ_TIMEOUT"`
	WriteTimeout      time.Duration `toml:"write_timeout" yaml:"write_timeout" json:"writeTimeout" env:"WRITE_TIMEOUT"`
	ReadHeaderTimeout time.Duration `toml:"read_header_timeout" yaml:"read_header_timeout" json:"readHeaderTimeout" env:"READ_HEADER_TIMEOUT"`
	IdleTimeout       time.Duration `toml:"idle_timeout" yaml:"idle_timeout" json:"idleTimeout" env:"IDLE_TIMEOUT"`
	ShutdownTimeout   time.Duration `toml:"shutdown_timeout" yaml:"shutdown_timeout" json:"shutdownTimeout" env:"SHUTDOWN_TIMEOUT" default:"30s"`
	MaxHeaderBytes    int           `toml:"max_header_bytes" yaml:"max_header_bytes" json:"maxHeaderBytes" env:"MAX_HEADER_BYTES"`
}

// INetwork defines an interface for network configuration,
// including address retrieval and base network access.
type INetwork interface {
	Addr() string
	Base() Network
}

// ErrPointerExpected is returned when a non-pointer or non-struct value is passed to setAppSettings.
const ErrPointerExpected bones.Error = "expected a pointer to a struct"

// Base returns the underlying Network value.
func (c Network) Base() Network { return c }

// PrepareTLSConfig builds a tls.Config from BaseGRPC.TLSConfig.
func (c Network) PrepareTLSConfig() (*tls.Config, error) {
	if c.TLSConfig == nil {
		return nil, ErrTLSDisabled
	}

	return c.TLSConfig.Prepare()
}

func (s *appSettings) SetAppNameAndVersion(name, version string) {
	s.name = name
	s.version = version
}

// AppName of the application from settings.
func (s *appSettings) AppName() string { return s.name }

// AppVersion of the application from settings.
func (s *appSettings) AppVersion() string { return s.version }

// setAppSettings propagates application name and version to known config sections.
//
// This is an internal helper with intentionally narrow reflection support:
// it recursively traverses nested struct fields starting from the root config
// value and applies metadata to fields implementing appSetter. Pointer fields,
// interface values, slices, maps, and other collection-like shapes are
// intentionally ignored. Unsupported shapes are skipped rather than treated as
// errors.
func setAppSettings(v any, name, version string) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%w: %T", ErrPointerExpected, v)
	}

	setAppSettingsRecursive(val.Elem(), name, version)

	return nil
}

// setAppSettingsRecursive walks supported nested struct fields and applies
// app metadata to values implementing appSetter. It intentionally skips
// non-struct fields and unsupported shapes such as pointers and collections.
func setAppSettingsRecursive(val reflect.Value, name, version string) {
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.Kind() != reflect.Struct || !field.IsValid() || !field.CanInterface() ||
			!field.CanAddr() {
			continue
		}

		setter, ok := field.Addr().Interface().(appSetter)
		if ok {
			setter.SetAppNameAndVersion(name, version)
		}

		setAppSettingsRecursive(field, name, version)
	}
}
