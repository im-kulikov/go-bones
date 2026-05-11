package network

import (
	"context"
	"net"
)

// ListenOpener abstracts listener creation for the gRPC server.
// It exists mainly to keep listener startup and failure paths testable.
type ListenOpener interface {
	Listen(ctx context.Context, network, address string) (net.Listener, error)
}
