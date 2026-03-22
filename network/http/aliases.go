package http

import "net/http"

// Handler aliases net/http.Handler for package-local use.
type Handler = http.Handler

// HandlerFunc aliases net/http.HandlerFunc for package-local use.
type HandlerFunc = http.HandlerFunc

// Request aliases net/http.Request for package-local use.
type Request = http.Request

// ResponseWriter aliases net/http.ResponseWriter for package-local use.
type ResponseWriter = http.ResponseWriter

// Header aliases net/http.Header for package-local use.
type Header = http.Header

// Client aliases net/http.Client for package-local use.
type Client = http.Client

// Transport aliases net/http.Transport for package-local use.
type Transport = http.Transport

// Server aliases net/http.Server for package-local use.
type Server = http.Server

// ServeMux aliases net/http.ServeMux for package-local use.
type ServeMux = http.ServeMux

const (
	// HTTP status code aliases from net/http for package-local use.

	// StatusContinue aliases net/http.StatusContinue for package-local use.
	StatusContinue = http.StatusContinue
	// StatusSwitchingProtocols aliases net/http.StatusSwitchingProtocols for package-local use.
	StatusSwitchingProtocols = http.StatusSwitchingProtocols
	// StatusProcessing aliases net/http.StatusProcessing for package-local use.
	StatusProcessing = http.StatusProcessing
	// StatusEarlyHints aliases net/http.StatusEarlyHints for package-local use.
	StatusEarlyHints = http.StatusEarlyHints

	// StatusOK aliases net/http.StatusOK for package-local use.
	StatusOK = http.StatusOK
	// StatusCreated aliases net/http.StatusCreated for package-local use.
	StatusCreated = http.StatusCreated
	// StatusAccepted aliases net/http.StatusAccepted for package-local use.
	StatusAccepted = http.StatusAccepted
	// StatusNonAuthoritativeInfo aliases net/http.StatusNonAuthoritativeInfo for package-local use.
	StatusNonAuthoritativeInfo = http.StatusNonAuthoritativeInfo
	// StatusNoContent aliases net/http.StatusNoContent for package-local use.
	StatusNoContent = http.StatusNoContent
	// StatusResetContent aliases net/http.StatusResetContent for package-local use.
	StatusResetContent = http.StatusResetContent
	// StatusPartialContent aliases net/http.StatusPartialContent for package-local use.
	StatusPartialContent = http.StatusPartialContent
	// StatusMultiStatus aliases net/http.StatusMultiStatus for package-local use.
	StatusMultiStatus = http.StatusMultiStatus
	// StatusAlreadyReported aliases net/http.StatusAlreadyReported for package-local use.
	StatusAlreadyReported = http.StatusAlreadyReported
	// StatusIMUsed aliases net/http.StatusIMUsed for package-local use.
	StatusIMUsed = http.StatusIMUsed

	// StatusMultipleChoices aliases net/http.StatusMultipleChoices for package-local use.
	StatusMultipleChoices = http.StatusMultipleChoices
	// StatusMovedPermanently aliases net/http.StatusMovedPermanently for package-local use.
	StatusMovedPermanently = http.StatusMovedPermanently
	// StatusFound aliases net/http.StatusFound for package-local use.
	StatusFound = http.StatusFound
	// StatusSeeOther aliases net/http.StatusSeeOther for package-local use.
	StatusSeeOther = http.StatusSeeOther
	// StatusNotModified aliases net/http.StatusNotModified for package-local use.
	StatusNotModified = http.StatusNotModified
	// StatusUseProxy aliases net/http.StatusUseProxy for package-local use.
	StatusUseProxy = http.StatusUseProxy
	// StatusTemporaryRedirect aliases net/http.StatusTemporaryRedirect for package-local use.
	StatusTemporaryRedirect = http.StatusTemporaryRedirect
	// StatusPermanentRedirect aliases net/http.StatusPermanentRedirect for package-local use.
	StatusPermanentRedirect = http.StatusPermanentRedirect

	// StatusBadRequest aliases net/http.StatusBadRequest for package-local use.
	StatusBadRequest = http.StatusBadRequest
	// StatusUnauthorized aliases net/http.StatusUnauthorized for package-local use.
	StatusUnauthorized = http.StatusUnauthorized
	// StatusPaymentRequired aliases net/http.StatusPaymentRequired for package-local use.
	StatusPaymentRequired = http.StatusPaymentRequired
	// StatusForbidden aliases net/http.StatusForbidden for package-local use.
	StatusForbidden = http.StatusForbidden
	// StatusNotFound aliases net/http.StatusNotFound for package-local use.
	StatusNotFound = http.StatusNotFound
	// StatusMethodNotAllowed aliases net/http.StatusMethodNotAllowed for package-local use.
	StatusMethodNotAllowed = http.StatusMethodNotAllowed
	// StatusNotAcceptable aliases net/http.StatusNotAcceptable for package-local use.
	StatusNotAcceptable = http.StatusNotAcceptable
	// StatusProxyAuthRequired aliases net/http.StatusProxyAuthRequired for package-local use.
	StatusProxyAuthRequired = http.StatusProxyAuthRequired
	// StatusRequestTimeout aliases net/http.StatusRequestTimeout for package-local use.
	StatusRequestTimeout = http.StatusRequestTimeout
	// StatusConflict aliases net/http.StatusConflict for package-local use.
	StatusConflict = http.StatusConflict
	// StatusGone aliases net/http.StatusGone for package-local use.
	StatusGone = http.StatusGone
	// StatusLengthRequired aliases net/http.StatusLengthRequired for package-local use.
	StatusLengthRequired = http.StatusLengthRequired
	// StatusPreconditionFailed aliases net/http.StatusPreconditionFailed for package-local use.
	StatusPreconditionFailed = http.StatusPreconditionFailed
	// StatusRequestEntityTooLarge aliases net/http.StatusRequestEntityTooLarge for package-local use.
	StatusRequestEntityTooLarge = http.StatusRequestEntityTooLarge
	// StatusRequestURITooLong aliases net/http.StatusRequestURITooLong for package-local use.
	StatusRequestURITooLong = http.StatusRequestURITooLong
	// StatusUnsupportedMediaType aliases net/http.StatusUnsupportedMediaType for package-local use.
	StatusUnsupportedMediaType = http.StatusUnsupportedMediaType
	// StatusRequestedRangeNotSatisfiable aliases net/http.StatusRequestedRangeNotSatisfiable for package-local use.
	StatusRequestedRangeNotSatisfiable = http.StatusRequestedRangeNotSatisfiable
	// StatusExpectationFailed aliases net/http.StatusExpectationFailed for package-local use.
	StatusExpectationFailed = http.StatusExpectationFailed
	// StatusTeapot aliases net/http.StatusTeapot for package-local use.
	StatusTeapot = http.StatusTeapot
	// StatusMisdirectedRequest aliases net/http.StatusMisdirectedRequest for package-local use.
	StatusMisdirectedRequest = http.StatusMisdirectedRequest
	// StatusUnprocessableEntity aliases net/http.StatusUnprocessableEntity for package-local use.
	StatusUnprocessableEntity = http.StatusUnprocessableEntity
	// StatusLocked aliases net/http.StatusLocked for package-local use.
	StatusLocked = http.StatusLocked
	// StatusFailedDependency aliases net/http.StatusFailedDependency for package-local use.
	StatusFailedDependency = http.StatusFailedDependency
	// StatusTooEarly aliases net/http.StatusTooEarly for package-local use.
	StatusTooEarly = http.StatusTooEarly
	// StatusUpgradeRequired aliases net/http.StatusUpgradeRequired for package-local use.
	StatusUpgradeRequired = http.StatusUpgradeRequired
	// StatusPreconditionRequired aliases net/http.StatusPreconditionRequired for package-local use.
	StatusPreconditionRequired = http.StatusPreconditionRequired
	// StatusTooManyRequests aliases net/http.StatusTooManyRequests for package-local use.
	StatusTooManyRequests = http.StatusTooManyRequests
	// StatusRequestHeaderFieldsTooLarge aliases net/http.StatusRequestHeaderFieldsTooLarge for package-local use.
	StatusRequestHeaderFieldsTooLarge = http.StatusRequestHeaderFieldsTooLarge
	// StatusUnavailableForLegalReasons aliases net/http.StatusUnavailableForLegalReasons for package-local use.
	StatusUnavailableForLegalReasons = http.StatusUnavailableForLegalReasons

	// StatusInternalServerError aliases net/http.StatusInternalServerError for package-local use.
	StatusInternalServerError = http.StatusInternalServerError
	// StatusNotImplemented aliases net/http.StatusNotImplemented for package-local use.
	StatusNotImplemented = http.StatusNotImplemented
	// StatusBadGateway aliases net/http.StatusBadGateway for package-local use.
	StatusBadGateway = http.StatusBadGateway
	// StatusServiceUnavailable aliases net/http.StatusServiceUnavailable for package-local use.
	StatusServiceUnavailable = http.StatusServiceUnavailable
	// StatusGatewayTimeout aliases net/http.StatusGatewayTimeout for package-local use.
	StatusGatewayTimeout = http.StatusGatewayTimeout
	// StatusHTTPVersionNotSupported aliases net/http.StatusHTTPVersionNotSupported for package-local use.
	StatusHTTPVersionNotSupported = http.StatusHTTPVersionNotSupported
	// StatusVariantAlsoNegotiates aliases net/http.StatusVariantAlsoNegotiates for package-local use.
	StatusVariantAlsoNegotiates = http.StatusVariantAlsoNegotiates
	// StatusInsufficientStorage aliases net/http.StatusInsufficientStorage for package-local use.
	StatusInsufficientStorage = http.StatusInsufficientStorage
	// StatusLoopDetected aliases net/http.StatusLoopDetected for package-local use.
	StatusLoopDetected = http.StatusLoopDetected
	// StatusNotExtended aliases net/http.StatusNotExtended for package-local use.
	StatusNotExtended = http.StatusNotExtended
	// StatusNetworkAuthenticationRequired aliases net/http.StatusNetworkAuthenticationRequired for package-local use.
	StatusNetworkAuthenticationRequired = http.StatusNetworkAuthenticationRequired

	// HTTP method aliases from net/http for package-local use.

	// MethodGet aliases net/http.MethodGet for package-local use.
	MethodGet = http.MethodGet
	// MethodHead aliases net/http.MethodHead for package-local use.
	MethodHead = http.MethodHead
	// MethodPost aliases net/http.MethodPost for package-local use.
	MethodPost = http.MethodPost
	// MethodPut aliases net/http.MethodPut for package-local use.
	MethodPut = http.MethodPut
	// MethodPatch aliases net/http.MethodPatch for package-local use.
	MethodPatch = http.MethodPatch
	// MethodDelete aliases net/http.MethodDelete for package-local use.
	MethodDelete = http.MethodDelete
	// MethodConnect aliases net/http.MethodConnect for package-local use.
	MethodConnect = http.MethodConnect
	// MethodOptions aliases net/http.MethodOptions for package-local use.
	MethodOptions = http.MethodOptions
	// MethodTrace aliases net/http.MethodTrace for package-local use.
	MethodTrace = http.MethodTrace
)

// nolint:gochecknoglobals
var (
	// Error aliases net/http.Error for package-local use.
	Error = http.Error

	// DefaultServeMux aliases net/http.DefaultServeMux for package-local use.
	// nolint: gochecknoglobals
	DefaultServeMux = http.DefaultServeMux

	// DefaultClient aliases net/http.DefaultClient for package-local use.
	DefaultClient = http.DefaultClient

	// ErrServerClosed aliases net/http.ErrServerClosed for package-local use.
	ErrServerClosed = http.ErrServerClosed

	// NoBody aliases net/http.NoBody for package-local use.
	NoBody = http.NoBody

	// NewRequestWithContext aliases net/http.NewRequestWithContext for package-local use.
	NewRequestWithContext = http.NewRequestWithContext
)

// NewServeMux returns a new net/http ServeMux.
func NewServeMux() *ServeMux { return http.NewServeMux() }
