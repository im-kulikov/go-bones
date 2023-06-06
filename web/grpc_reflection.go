package web

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type reflectionService struct{}

const reflectionServiceName = "grpc-reflection"

var _ GRPCService = (*reflectionService)(nil)

// Name of the reflection service.
func (reflectionService) Name() string { return reflectionServiceName }

// Register reflection service.
func (reflectionService) Register(srv *grpc.Server) { reflection.Register(srv) }
