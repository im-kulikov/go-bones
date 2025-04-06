package config

import (
	"testing"
)

type TestConfig struct {
	Base
}

func inner(config Base) {}

func Test_testing(t *testing.T) {
	var config TestConfig

	inner(config.Base)
}
