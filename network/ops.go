package network

import (
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

const defaultOPSServiceName = "ops"

func NewOPSServer(cfg config.Ops, log *logger.Logger) (service.Service, error) {
	mux := http.NewServeMux()

	// prepare metrics handlers
	mux.Handle(cfg.MetricsPath, promhttp.Handler())

	// prepare pprof handler
	mux.HandleFunc(cfg.ProfilePath+"/", pprof.Index)
	mux.HandleFunc(cfg.ProfilePath+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(cfg.ProfilePath+"/profile", pprof.Profile)
	mux.HandleFunc(cfg.ProfilePath+"/symbol", pprof.Symbol)
	mux.HandleFunc(cfg.ProfilePath+"/trace", pprof.Trace)

	return NewHTTPServer(cfg, log, mux, HTTPServiceName(defaultOPSServiceName))
}
