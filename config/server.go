package config

// HTTP is a ready-made config section for an HTTP server built with
// network/http.NewServer: an address plus the common network settings.
//
//	type settings struct {
//		config.Base
//		API config.HTTP `env:"API" yaml:"api"`
//	}
//
// nolint:lll
type HTTP struct {
	Network

	Address string `yaml:"address" env:"ADDRESS" toml:"address" json:"address" default:":8080"`
}

// Addr implements INetwork.
func (c HTTP) Addr() string { return c.Address }

// GRPC is a ready-made config section for a gRPC server built with
// network/grpc.NewServer: an address plus the common network settings.
//
// nolint:lll
type GRPC struct {
	Network

	Address string `yaml:"address" env:"ADDRESS" toml:"address" json:"address" default:":9090"`
}

// Addr implements INetwork.
func (c GRPC) Addr() string { return c.Address }
