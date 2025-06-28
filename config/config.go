package config

import (
	"github.com/im-kulikov/gonfig"
)

type settings struct {
	appSettings

	kind gonfig.ParserType

	options []gonfig.LoaderOption
}

// Option allows to customize configuration.
type Option func(*settings)

// WithName sets application name.
func WithName(name string) Option { return func(s *settings) { s.name = name } }

// WithVersion sets application version.
func WithVersion(version string) Option { return func(s *settings) { s.version = version } }

func WithParsers(loaders ...gonfig.Parser) Option {
	return func(s *settings) {
		for _, loader := range loaders {
			s.options = append(s.options, gonfig.WithCustomParser(loader))
		}
	}
}

func WithParserInt(prepares ...gonfig.ParserInit) Option {
	return func(s *settings) {
		for _, preparer := range prepares {
			s.options = append(s.options, gonfig.WithCustomParserInit(preparer))
		}
	}
}

func WithLoaderOptions(options ...gonfig.LoaderOption) Option {
	return func(s *settings) { s.options = append(s.options, options...) }
}

func WithCustomizeLoaderConfig(handler func(*gonfig.Config)) Option {
	return func(s *settings) { s.options = append(s.options, gonfig.WithConfig(handler)) }
}

func WithYAML() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserYAML
		s.options = append(s.options, gonfig.WithYAMLLoader())
	}
}

func WithJSON() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserJSON
		s.options = append(s.options, gonfig.WithJSONLoader())
	}
}

func WithTOML() Option {
	return func(s *settings) {
		s.kind = gonfig.ParserTOML
		s.options = append(s.options, gonfig.WithTOMLLoader())
	}
}

func Load(v any, options ...Option) error {
	var cfg settings

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
