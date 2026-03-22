package config

import (
	"github.com/im-kulikov/gonfig"
)

// DefaultConfigFlag is a zero-size structure that can be embedded into
// configuration structures to automatically enable default --config and -c flags.
type DefaultConfigFlag = gonfig.DefaultConfigFlag
