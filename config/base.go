package config

import (
	"fmt"
	"reflect"

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

// ErrPointerExpected is returned when a non-pointer or non-struct value is passed to setAppSettings.
const ErrPointerExpected bones.Error = "expected a pointer to a struct"

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
