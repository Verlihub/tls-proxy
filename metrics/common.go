package metrics

type Counter interface {
	Add(v float64)
}

type Gauge interface {
	Counter
}

type Observer interface {
	Observe(v float64)
}

var (
	_ Counter  = noop{}
	_ Gauge    = noop{}
	_ Observer = noop{}
)

type noop struct{}

func (noop) Add(v float64)     {}
func (noop) Observe(v float64) {}

func ListenAndServe(addr string) error {
	return listenAndServe(addr)
}

var (
	ConnAccepted     Counter  = noop{}
	ConnError        Counter  = noop{}
	ConnOpen         Gauge    = noop{}
	ConnInsecure     Counter  = noop{}
	ConnOpenInsecure Gauge    = noop{}
	ConnTLS          Counter  = noop{}
	ConnOpenTLS      Gauge    = noop{}
	ConnTLSHandshake Observer = noop{}
	ConnRx           Counter  = noop{}
	ConnTx           Counter  = noop{}
)
