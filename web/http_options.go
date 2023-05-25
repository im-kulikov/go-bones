package web

import (
	"net/http"

	"github.com/im-kulikov/go-bones/logger"
)

// HTTPOption allows customizing http component settings.
type HTTPOption func(*httpServer)

// WithHTTPConfig allows set custom http settings.
func WithHTTPConfig(v HTTPConfig) HTTPOption {
	return func(s *httpServer) { s.HTTPConfig = v }
}

// WithHTTPName allows set custom http name value.
func WithHTTPName(v string) HTTPOption {
	return func(s *httpServer) { s.name = v }
}

// WithHTTPLogger allows set custom logger value.
func WithHTTPLogger(v logger.Logger) HTTPOption {
	return func(s *httpServer) { s.logger = v }
}

// WithHTTPHandler allows set custom http.Handler value.
func WithHTTPHandler(v http.Handler) HTTPOption {
	return func(s *httpServer) { s.handle = v }
}
