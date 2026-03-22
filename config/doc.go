// Package config defines shared configuration types and loading helpers for go-bones services.
//
// It also exposes helpers such as DefaultConfigFlag for enabling the standard
// --config and -c file path flags in application config structs.
//
// Example:
//
//	type appConfig struct {
//		Base `env:",squash" yaml:",inline" json:",inline" toml:",inline"`
//		DefaultConfigFlag `env:",squash" yaml:",inline" json:",inline" toml:",inline"`
//
//		HTTP struct {
//			Address string `env:"ADDRESS" yaml:"address" default:":8080"`
//		} `env:"HTTP" yaml:"http" json:"http" toml:"http"`
//	}
package config
