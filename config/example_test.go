package config_test

import (
	"fmt"

	"github.com/im-kulikov/go-bones/config"
)

// Defaults builds a section as Load would with no other sources: a struct
// literal would have every *_enabled switch false.
func ExampleDefaults() {
	ops := config.Defaults[config.Ops]()
	ops.Address = "127.0.0.1:0"

	fmt.Println(ops.Enabled, ops.MetricsEnabled, ops.ReadyPath)

	// Output: true true /readyz
}
