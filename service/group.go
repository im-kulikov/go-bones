package service

import "context"

// Group allows to provide runner of services
type Group struct {
	name string
	opts []Service
}

// Name used to implement Service interface.
func (g *Group) Name() string { return g.name }

// Stop used to implement Service interface.
func (g *Group) Stop(context.Context) {}

// Start used to implement Service interface.
func (g *Group) Start(context.Context) error { return nil }

// Services return multiple services.
func (g *Group) Services() []Service { return g.opts }

// NewGroup returns a group of services.
func NewGroup(name string, services ...Service) Service {
	out := &Group{name: name, opts: make([]Service, 0, len(services))}
	for _, svc := range services {
		if svc == nil {
			continue
		}

		if e, ok := svc.(Enabler); ok && !e.Enabled() {
			continue
		}

		out.opts = append(out.opts, svc)
	}

	return out
}
