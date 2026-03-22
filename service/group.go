package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/im-kulikov/go-bones"
)

// ErrComposedServiceNotRunnable is returned when attempting to start a composed service directly.
const ErrComposedServiceNotRunnable bones.Error = "composed service is not runnable"

// composed is a lightweight container used to pass multiple services through APIs
// that accept a single Service value.
//
// It intentionally satisfies Service only as a transport shape for composition:
// orchestration helpers unwrap it, but the container itself does not own a runnable
// lifecycle.
type composed []Service

// Name returns a string listing all service names in the composed service.
func (g composed) Name() string {
	services := make([]string, 0, len(g))
	for _, service := range g {
		services = append(services, service.Name())
	}

	return fmt.Sprintf("composed-services(%s)", strings.Join(services, ","))
}

// Stop implements Service, but composed services do not manage their own lifecycle.
// It is a no-op because composed services are intended only for service composition.
func (g composed) Stop(context.Context) {}

// Start implements Service, but composed services are not intended to be started directly.
// Use Compose only as a container for service composition, for example with WithService.
func (g composed) Start(context.Context) error {
	return ErrComposedServiceNotRunnable
}

// Compose constructs a composed service from the provided services.
//
// Why it returns Service instead of []Service:
//   - existing option helpers accept Service values;
//   - a composed wrapper lets callers pass a group through the same surface;
//   - service execution still happens only after helpers unwrap the container.
//
// Nil services and disabled services (for types implementing Enabler) are excluded
// so callers can assemble optional trees without extra filtering code.
func Compose(services ...Service) Service {
	out := make(composed, 0, len(services))
	for _, svc := range services {
		if svc == nil {
			continue
		}

		if e, ok := svc.(Enabler); ok && !e.Enabled() {
			continue
		}

		out = append(out, svc)
	}

	return out
}
