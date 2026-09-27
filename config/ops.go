package config

// Ops contains settings for OPS server.
// nolint:lll
type Ops struct {
	Network `yaml:",inline" env:",squash"`

	Address string `yaml:"address" env:"ADDRESS" toml:"address" json:"address" default:":8090"`
	Enabled bool   `yaml:"enabled" env:"ENABLED" toml:"enabled" json:"enabled" default:"true"`

	MetricsPath    string `yaml:"metrics_path"    env:"METRICS_PATH"    toml:"metrics_path"    json:"metrics_path"    default:"/metrics"`
	MetricsEnabled bool   `yaml:"metrics_enabled" env:"METRICS_ENABLED" toml:"metrics_enabled" json:"metrics_enabled" default:"true"`

	ProfilePath    string `yaml:"profile_path"    env:"PROFILE_PATH"    toml:"profile_path"    json:"profile_path"    default:"/debug/pprof"`
	ProfileEnabled bool   `yaml:"profile_enabled" env:"PROFILE_ENABLED" toml:"profile_enabled" json:"profile_enabled" default:"true"`

	ExpVarsPath    string `yaml:"exp_vars_path"    env:"EXP_VARS_PATH"    toml:"exp_vars_path"    json:"exp_vars_path"    default:"/debug/vars"`
	ExpVarsEnabled bool   `yaml:"exp_vars_enabled" env:"EXP_VARS_ENABLED" toml:"exp_vars_enabled" json:"exp_vars_enabled" default:"true"`

	VersionPath    string `yaml:"version_path"    env:"VERSION_PATH"    toml:"version_path"    json:"version_path"    default:"/version"`
	VersionEnabled bool   `yaml:"version_enabled" env:"VERSION_ENABLED" toml:"version_enabled" json:"version_enabled" default:"false"`
}

// Addr returns the configured listen address.
func (c Ops) Addr() string { return c.Address }

// IsEnabled reports whether the OPS server and at least one of its endpoints are enabled.
func (c Ops) IsEnabled() bool {
	return c.Enabled &&
		(c.MetricsEnabled || c.ProfileEnabled || c.ExpVarsEnabled || c.VersionEnabled)
}
