package config

import (
	"fmt"

	"github.com/im-kulikov/gonfig"
)

// Defaults returns a T with its `default` tags applied, without reading files,
// env or flags. Use it to build a config section in code or tests: a literal
// such as Ops{} has every *_enabled switch false, while Defaults[Ops]() matches
// what Load produces with no other sources.
//
//	cfg := config.Defaults[config.Ops]()
//	cfg.Address = "127.0.0.1:0"
//
// It panics if a default tag cannot be parsed: that is a bug in T, not a
// runtime condition.
func Defaults[T any]() T {
	var v T
	if err := gonfig.SetDefaults(&v); err != nil {
		panic(fmt.Errorf("config: defaults of %T: %w", v, err))
	}

	return v
}
