package service

import (
	"context"
	"fmt"
	"strings"
)

// group is a collection of services that implements the Service interface.
type group []Service

// Name returns a string listing all service names in the group.
func (g group) Name() string {
	services := make([]string, 0, len(g))
	for _, service := range g {
		services = append(services, service.Name())
	}

	return fmt.Sprintf("group-of-services(%s)", strings.Join(services, ","))
}

// Stop is not intended to be called for a group; it panics if invoked.
func (g group) Stop(context.Context) {
	panic("should not be called")
}

// Start is not intended to be called for a group; it panics if invoked.
func (g group) Start(context.Context) error {
	panic("should not be called")
}

// NewGroup constructs a group from the provided services. Nil services or those
// disabled (when implementing Enabler) are excluded. The group returned also
// implements the Service interface.
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
