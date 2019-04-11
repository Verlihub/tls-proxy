// +build metrics

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func listenAndServe(addr string) error {
	return http.ListenAndServe(addr, promhttp.Handler())
}

func init() {
	ConnAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_accepted",
		Help: "The total number of accepted connections",
	})
	ConnError = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_error",
		Help: "The total number of connections failed with an error",
	})
	ConnOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dc_conn_open",
		Help: "The number of open connections",
	})
	ConnInsecure = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_insecure",
		Help: "The total number of insecure connections",
	})
	ConnOpenInsecure = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dc_conn_insecure_open",
		Help: "The number of open insecure connections",
	})
	ConnTLS = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_tls",
		Help: "The total number of TLS connections",
	})
	ConnOpenTLS = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dc_conn_tls_open",
		Help: "The number of open TLS connections",
	})
	ConnTLSHandshake = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "dc_conn_tls_handshake_sec",
		Help: "Time spent on TLS handshake",
	})
	ConnRx = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_rx_bytes",
		Help: "Total bytes received from the client",
	})
	ConnTx = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dc_conn_tx_bytes",
		Help: "Total bytes sent to the client",
	})
}
