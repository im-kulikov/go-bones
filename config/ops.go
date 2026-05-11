package config

// Ops contains settings for OPS server.
// nolint:lll
type Ops struct {
	Network `yaml:",inline" env:",squash"`

	Address        string `yaml:"address"         env:"ADDRESS"         toml:"address"         json:"address"         default:":8090"`
	MetricsPath    string `yaml:"metrics_path"    env:"METRICS_PATH"    toml:"metrics_path"    json:"metrics_path"    default:"/metrics"`
	ProfilePath    string `yaml:"profile_path"    env:"PROFILE_PATH"    toml:"profile_path"    json:"profile_path"    default:"/debug/pprof"`
	ExpVarsPath    string `yaml:"exp_vars_path"   env:"EXP_VARS_PATH"   toml:"exp_vars_path"   json:"exp_vars_path"   default:"/debug/vars"`
	VersionPath    string `yaml:"version_path"    env:"VERSION_PATH"    toml:"version_path"    json:"version_path"    default:"/version"`
	VersionEnabled bool   `yaml:"version_enabled" env:"VERSION_ENABLED" toml:"version_enabled" json:"version_enabled" default:"false"`
}

// Addr returns the configured listen address.
func (c Ops) Addr() string { return c.Address }
