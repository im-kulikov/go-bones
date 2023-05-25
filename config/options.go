package config

import "io"

// Option allows to set custom settings.
type Option func(*config)

type config struct {
	envPath string
	version string

	fatalf func(string, ...interface{})

	out io.Writer
	pwd func() (string, error)

	args []string
	envs []string
	exit func(int)

	showHelp bool
	showCurr bool
	validate bool
	markdown bool
}

// WithVersion allows to set current version.
func WithVersion(v string) Option {
	return func(c *config) { c.version = v }
}

// WithEnvPath allows to set environment file path.
func WithEnvPath(v string) Option {
	return func(c *config) { c.envPath = v }
}

// WithArgs allows to set custom os.Args.
func WithArgs(v []string) Option {
	return func(c *config) { c.args = v }
}

// WithEnvs allows to set custom ENVs.
func WithEnvs(v []string) Option {
	return func(c *config) { c.envs = v }
}
