package config

import (
	"runtime/debug"

	"github.com/im-kulikov/gonfig"
)

type settings struct {
	appSettings

	kind gonfig.ParserType

	options []gonfig.LoaderOption
}

// Option allows customizing configuration.
type Option func(*settings)

// WithName sets the application name.
func WithName(name string) Option { return func(s *settings) { s.name = name } }

// WithVersion sets the application version.
func WithVersion(version string) Option { return func(s *settings) { s.version = version } }

// WithParsers registers custom parsers in gonfig loader options.
func WithParsers(loaders ...gonfig.Parser) Option {
	return func(s *settings) {
		for _, loader := range loaders {
			s.options = append(s.options, gonfig.WithCustomParser(loader))
		}
	}
}

// WithParserInit registers custom parser initializers in gonfig loader options.
func WithParserInit(prepares ...gonfig.ParserInit) Option {
	return func(s *settings) {
		for _, preparer := range prepares {
			s.options = append(s.options, gonfig.WithCustomParserInit(preparer))
		}
	}
}

// WithLoaderOptions appends raw gonfig loader options.
func WithLoaderOptions(options ...gonfig.LoaderOption) Option {
	return func(s *settings) { s.options = append(s.options, options...) }
}

// WithCustomizeLoaderConfig registers a callback that mutates gonfig.Config before loading.
func WithCustomizeLoaderConfig(handler func(*gonfig.Config)) Option {
	return func(s *settings) { s.options = append(s.options, gonfig.WithConfig(handler)) }
}

// WithYAML enables YAML parsing and makes it the preferred parser kind.
func WithYAML() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserYAML
		s.options = append(s.options, gonfig.WithYAMLLoader())
	}
}

// WithJSON enables JSON parsing and makes it the preferred parser kind.
func WithJSON() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserJSON
		s.options = append(s.options, gonfig.WithJSONLoader())
	}
}

// WithTOML enables TOML parsing and makes it the preferred parser kind.
func WithTOML() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserTOML
		s.options = append(s.options, gonfig.WithTOMLLoader())
	}
}

func (c *settings) setDefaults() {
	if info, ok := debug.ReadBuildInfo(); ok {
		c.name = info.Main.Path
		c.version = info.Main.Version
	}
}

// Load fills v from configuration sources configured through Option values.
// YAML loading is enabled by default when no parser option was supplied.
func Load(v any, options ...Option) error {
	var cfg settings

	cfg.setDefaults()
	for _, option := range options {
		option(&cfg)
	}

	if cfg.kind == "" { // enable yaml by default
		cfg.kind = gonfig.ParserYAML
		cfg.options = append(cfg.options, gonfig.WithYAMLLoader())
	}

	if err := gonfig.Load(v, cfg.options...); err != nil {
		return err
	}

	return setAppSettings(v, cfg.name, cfg.version)
}
