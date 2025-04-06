package service

import (
	"context"
	"fmt"
	"strings"
)

type group []Service

// Name used to implement Service interface.
func (g group) Name() string {
	services := make([]string, 0, len(g))
	for _, service := range g {
		services = append(services, service.Name())
	}

	return fmt.Sprintf("group-of-services(%s)", strings.Join(services, ","))
}

// Stop used to implement Service interface.
func (g group) Stop(context.Context) { panic("should not be called") }

// Start used to implement Service interface.
func (g group) Start(context.Context) error { panic("should not be called") }

// NewGroup returns a group of services.
func NewGroup(services ...Service) Service {
	out := make(group, 0, len(services))
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
