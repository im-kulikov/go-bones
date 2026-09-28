// Package config defines shared configuration types and loading helpers for go-bones services.
//
// Base, embedded in the config of an application, also gives it the flags
// --config / -c (the path of the config file) and --print-config.
//
// Example:
//
//	type appConfig struct {
//		Base
//
//		HTTP struct {
//			Address string `env:"ADDRESS" yaml:"address" default:":8080"`
//		} `env:"HTTP" yaml:"http" json:"http" toml:"http"`
//	}
package config
