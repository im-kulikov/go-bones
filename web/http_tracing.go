package web

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ApplyTracingToHTTPClient propagate tracing.ID and X-Request-ID on exists http.Client.
func ApplyTracingToHTTPClient(cli *http.Client) {
	cli.Transport = otelhttp.NewTransport(cli.Transport)
}

// HTTPTracingMiddleware wraps the passed http.Handler, functioning like middleware, in a span
// named after the operation and with any provided Options.
func HTTPTracingMiddleware(handler http.Handler, opts ...otelhttp.Option) http.Handler {
	return otelhttp.NewHandler(handler, "", opts...)
}

// HTTPTracingMiddlewareFunc wraps the passed  http.HandlerFunc, functioning like middleware, in a span
// named after the operation and with any provided Options.
func HTTPTracingMiddlewareFunc(handler http.HandlerFunc, opts ...otelhttp.Option) http.Handler {
	return otelhttp.NewHandler(handler, "", opts...)
}
