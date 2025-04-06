package config

import (
	"fmt"
	"reflect"

	"github.com/im-kulikov/go-bones"
)

type Base struct {
	Logger    Logger `env:"LOGGER" yaml:"logger" json:"logger" toml:"logger"`
	OpsServer Ops    `env:"OPS"    yaml:"ops"    json:"ops"    toml:"ops"`
}

type appSettings struct {
	name    string
	version string
}

type appSetter interface {
	SetAppNameAndVersion(name, version string)
}

const ErrPointerExpected bones.Error = "expected a pointer to a struct"

var baseType = reflect.TypeOf(Base{}) // nolint:gochecknoglobals

func (s *appSettings) SetAppNameAndVersion(name, version string) {
	s.name = name
	s.version = version
}

// AppName of the application from settings.
func (s *appSettings) AppName() string { return s.name }

// AppVersion of the application from settings.
func (s *appSettings) AppVersion() string { return s.version }

func setAppSettings(v any, name, version string) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%w: %T", ErrPointerExpected, v)
	}

	base := val.Elem().FieldByName("Base")
	if base.IsValid() && base.CanConvert(baseType) {
		setAppSettingsRecursive(base, name, version)
	}

	return nil
}

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
