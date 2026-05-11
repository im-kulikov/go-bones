package http

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPStatusAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "StatusContinue", got: StatusContinue, want: 100},
		{name: "StatusSwitchingProtocols", got: StatusSwitchingProtocols, want: 101},
		{name: "StatusProcessing", got: StatusProcessing, want: 102},
		{name: "StatusEarlyHints", got: StatusEarlyHints, want: 103},
		{name: "StatusOK", got: StatusOK, want: 200},
		{name: "StatusCreated", got: StatusCreated, want: 201},
		{name: "StatusAccepted", got: StatusAccepted, want: 202},
		{name: "StatusNonAuthoritativeInfo", got: StatusNonAuthoritativeInfo, want: 203},
		{name: "StatusNoContent", got: StatusNoContent, want: 204},
		{name: "StatusResetContent", got: StatusResetContent, want: 205},
		{name: "StatusPartialContent", got: StatusPartialContent, want: 206},
		{name: "StatusMultiStatus", got: StatusMultiStatus, want: 207},
		{name: "StatusAlreadyReported", got: StatusAlreadyReported, want: 208},
		{name: "StatusIMUsed", got: StatusIMUsed, want: 226},
		{name: "StatusMultipleChoices", got: StatusMultipleChoices, want: 300},
		{name: "StatusMovedPermanently", got: StatusMovedPermanently, want: 301},
		{name: "StatusFound", got: StatusFound, want: 302},
		{name: "StatusSeeOther", got: StatusSeeOther, want: 303},
		{name: "StatusNotModified", got: StatusNotModified, want: 304},
		{name: "StatusUseProxy", got: StatusUseProxy, want: 305},
		{name: "StatusTemporaryRedirect", got: StatusTemporaryRedirect, want: 307},
		{name: "StatusPermanentRedirect", got: StatusPermanentRedirect, want: 308},
		{name: "StatusBadRequest", got: StatusBadRequest, want: 400},
		{name: "StatusUnauthorized", got: StatusUnauthorized, want: 401},
		{name: "StatusPaymentRequired", got: StatusPaymentRequired, want: 402},
		{name: "StatusForbidden", got: StatusForbidden, want: 403},
		{name: "StatusNotFound", got: StatusNotFound, want: 404},
		{name: "StatusMethodNotAllowed", got: StatusMethodNotAllowed, want: 405},
		{name: "StatusNotAcceptable", got: StatusNotAcceptable, want: 406},
		{name: "StatusProxyAuthRequired", got: StatusProxyAuthRequired, want: 407},
		{name: "StatusRequestTimeout", got: StatusRequestTimeout, want: 408},
		{name: "StatusConflict", got: StatusConflict, want: 409},
		{name: "StatusGone", got: StatusGone, want: 410},
		{name: "StatusLengthRequired", got: StatusLengthRequired, want: 411},
		{name: "StatusPreconditionFailed", got: StatusPreconditionFailed, want: 412},
		{name: "StatusRequestEntityTooLarge", got: StatusRequestEntityTooLarge, want: 413},
		{name: "StatusRequestURITooLong", got: StatusRequestURITooLong, want: 414},
		{name: "StatusUnsupportedMediaType", got: StatusUnsupportedMediaType, want: 415},
		{
			name: "StatusRequestedRangeNotSatisfiable",
			got:  StatusRequestedRangeNotSatisfiable,
			want: 416,
		},
		{name: "StatusExpectationFailed", got: StatusExpectationFailed, want: 417},
		{name: "StatusTeapot", got: StatusTeapot, want: 418},
		{name: "StatusMisdirectedRequest", got: StatusMisdirectedRequest, want: 421},
		{name: "StatusUnprocessableEntity", got: StatusUnprocessableEntity, want: 422},
		{name: "StatusLocked", got: StatusLocked, want: 423},
		{name: "StatusFailedDependency", got: StatusFailedDependency, want: 424},
		{name: "StatusTooEarly", got: StatusTooEarly, want: 425},
		{name: "StatusUpgradeRequired", got: StatusUpgradeRequired, want: 426},
		{name: "StatusPreconditionRequired", got: StatusPreconditionRequired, want: 428},
		{name: "StatusTooManyRequests", got: StatusTooManyRequests, want: 429},
		{
			name: "StatusRequestHeaderFieldsTooLarge",
			got:  StatusRequestHeaderFieldsTooLarge,
			want: 431,
		},
		{
			name: "StatusUnavailableForLegalReasons",
			got:  StatusUnavailableForLegalReasons,
			want: 451,
		},
		{name: "StatusInternalServerError", got: StatusInternalServerError, want: 500},
		{name: "StatusNotImplemented", got: StatusNotImplemented, want: 501},
		{name: "StatusBadGateway", got: StatusBadGateway, want: 502},
		{name: "StatusServiceUnavailable", got: StatusServiceUnavailable, want: 503},
		{name: "StatusGatewayTimeout", got: StatusGatewayTimeout, want: 504},
		{name: "StatusHTTPVersionNotSupported", got: StatusHTTPVersionNotSupported, want: 505},
		{name: "StatusVariantAlsoNegotiates", got: StatusVariantAlsoNegotiates, want: 506},
		{name: "StatusInsufficientStorage", got: StatusInsufficientStorage, want: 507},
		{name: "StatusLoopDetected", got: StatusLoopDetected, want: 508},
		{name: "StatusNotExtended", got: StatusNotExtended, want: 510},
		{
			name: "StatusNetworkAuthenticationRequired",
			got:  StatusNetworkAuthenticationRequired,
			want: 511,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestHTTPMethodAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "MethodGet", got: MethodGet, want: "GET"},
		{name: "MethodHead", got: MethodHead, want: "HEAD"},
		{name: "MethodPost", got: MethodPost, want: "POST"},
		{name: "MethodPut", got: MethodPut, want: "PUT"},
		{name: "MethodPatch", got: MethodPatch, want: "PATCH"},
		{name: "MethodDelete", got: MethodDelete, want: "DELETE"},
		{name: "MethodConnect", got: MethodConnect, want: "CONNECT"},
		{name: "MethodOptions", got: MethodOptions, want: "OPTIONS"},
		{name: "MethodTrace", got: MethodTrace, want: "TRACE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.got)
		})
	}
}
