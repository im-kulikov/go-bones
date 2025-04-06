package network

import (
	"net/http"

	"github.com/im-kulikov/go-bones/service"
)

type grpcSettings struct{}

type GRPCOption func(*grpcSettings)

func NewGRPCServer(handler http.Handler, opts ...GRPCOption) service.Service {
	panic("implement me")
}
