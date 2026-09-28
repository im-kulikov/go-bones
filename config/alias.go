package config

import (
	"github.com/im-kulikov/gonfig"
)

// DefaultConfigFlag enables the --config and -c flags: the path of the config
// file. Base embeds it, so a config with Base has them already.
type DefaultConfigFlag = gonfig.DefaultConfigFlag

// PrintConfigFlag enables --print-config[=yaml|json|toml|env]: it prints the
// loaded config, secrets left empty, and exits with code 0. Base embeds it.
type PrintConfigFlag = gonfig.PrintConfigFlag
