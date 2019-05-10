package dcproxy

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Verlihub/tls-proxy/metrics"
)

func init() {
	os.Setenv("GODEBUG", os.Getenv("GODEBUG")+",tls13=1")
}

type Config struct {
	HubAddr    string        // hub address (default = 127.0.0.1:411)
	HubNetwork string        // hub network (default = tcp4)
	Hosts      []string      // list of hosts to listen on
	Cert       string        // path to TLS cert file
	Key        string        // path to TLS key file
	PProf      string        // serve profiler on a given address (empty = disabled)
	Metrics    string        // serve metrics on a given address (empty = disabled)
	LogErrors  bool          // log connection errors
	Wait       time.Duration // time to wait for protocol detection
	Buffer     int           // buffer size in KB
	NoSendIP   bool          // don't send client's IP
}

type Proxy struct {
	c   Config
	tls *tls.Config

	wg  sync.WaitGroup
	lis []net.Listener
}

func New(c Config) (*Proxy, error) {
	if c.HubAddr == "" {
		c.HubNetwork = "tcp4"
		c.HubAddr = "127.0.0.1:411"
	} else if c.HubNetwork == "" {
		c.HubNetwork = "tcp4"
	}
	if c.Buffer == 0 {
		c.Buffer = 10
	}
	p := &Proxy{c: c}
	if c.Cert != "" && c.Key != "" {
		log.Println("using certs:", c.Cert, c.Key)
		var err error
		tlsConfig := &tls.Config{NextProtos: []string{"nmdc"}}
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			return nil, err
		}
		p.tls = tlsConfig
	} else {
		log.Println("no certs; TLS disabled")
	}
	return p, nil
}

func (p *Proxy) Wait() {
	p.wg.Wait()
}

func (p *Proxy) Close() error {
	for _, l := range p.lis {
		_ = l.Close()
	}
	p.lis = nil
	return nil
}

func (p *Proxy) Run() error {
	if p.c.PProf != "" {
		log.Println("enabling profiler on", p.c.PProf)

		go func() {
			if err := http.ListenAndServe(p.c.PProf, nil); err != nil {
				log.Println("cannot enable profiler:", err)
			}
		}()
	}
	if p.c.Metrics != "" {
		log.Println("serving metrics on", p.c.Metrics)

		go func() {
			if err := metrics.ListenAndServe(p.c.Metrics); err != nil {
				log.Println("cannot serve metrics:", err)
			}
		}()
	}

	for _, host := range p.c.Hosts {
		l, err := net.Listen("tcp4", host)

		if err != nil {
			_ = p.Close()
			return err
		}

		p.lis = append(p.lis, l)
	}

	for i, l := range p.lis {
		p.wg.Add(1)
		l := l
		log.Println("proxying", p.c.Hosts[i], "to", p.c.HubAddr)

		go func() {
			defer p.wg.Done()
			p.acceptOn(l)
		}()
	}
	return nil
}

func (p *Proxy) acceptOn(l net.Listener) {
	for {
		c, err := l.Accept()

		if err != nil {
			if p.c.LogErrors {
				log.Println(err)
			}

			metrics.ConnError.Add(1)
			continue
		}

		metrics.ConnAccepted.Add(1)

		go func() {
			err := p.serve(c)

			if err != nil && err != io.EOF {
				metrics.ConnError.Add(1)

				if p.c.LogErrors {
					log.Println(c.RemoteAddr(), err)
				}
			}
		}()
	}
}

type timeoutErr interface {
	Timeout() bool
}

func (p *Proxy) serve(c net.Conn) error {
	metrics.ConnOpen.Add(1)

	defer func() {
		_ = c.Close()
		metrics.ConnOpen.Add(-1)
	}()

	buf := make([]byte, 1024)
	i := copy(buf, "$MyIP ")
	i += copy(buf[i:], c.RemoteAddr().(*net.TCPAddr).IP.String())
	i += copy(buf[i:], " 0.0|")

	if p.tls == nil || p.c.Wait <= 0 { // no auto detection
		metrics.ConnInsecure.Add(1)
		metrics.ConnOpenInsecure.Add(1)
		defer metrics.ConnOpenInsecure.Add(-1)
		return p.writeAndStream(buf[:i], c, i)
	}

	err := c.SetReadDeadline(time.Now().Add(p.c.Wait))

	if err != nil {
		return err
	}

	start := time.Now()
	n, err := c.Read(buf[i:])
	_ = c.SetReadDeadline(time.Time{})

	if e, ok := err.(timeoutErr); ok && e.Timeout() { // has to be plain nmdc
		metrics.ConnInsecure.Add(1)
		metrics.ConnOpenInsecure.Add(1)
		defer metrics.ConnOpenInsecure.Add(-1)
		return p.writeAndStream(buf[:i], c, i)
	}

	if err != nil {
		return err
	}

	buf = buf[:i+n]

	if n >= 2 && buf[i] == 0x16 && buf[i+1] == 0x03 {
		tlsBuf := buf[i:]
		c = &multiReadConn{r: io.MultiReader(bytes.NewReader(tlsBuf), c), Conn: c}
		tc := tls.Server(c, p.tls) // tls handshake
		defer tc.Close()
		err = tc.Handshake()

		if err != nil {
			return err
		}

		buf[i-4] = '1' // set version
		state := tc.ConnectionState()

		switch state.Version {
		case tls.VersionTLS13:
			buf[i-2] = '3'
		case tls.VersionTLS12:
			buf[i-2] = '2'
		case tls.VersionTLS11:
			buf[i-2] = '1'
		}

		buf = buf[:i]
		dt := time.Since(start).Seconds()
		metrics.ConnTLS.Add(1)
		metrics.ConnOpenTLS.Add(1)
		defer metrics.ConnOpenTLS.Add(-1)
		metrics.ConnTLSHandshake.Observe(dt)
		return p.writeAndStream(buf, tc, i)
	}

	metrics.ConnInsecure.Add(1)
	metrics.ConnOpenInsecure.Add(1)
	defer metrics.ConnOpenInsecure.Add(-1)
	return p.writeAndStream(buf, c, i)
}

type multiReadConn struct {
	r io.Reader
	net.Conn
}

func (c *multiReadConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func (p *Proxy) dialHub() (net.Conn, error) {
	return net.Dial(p.c.HubNetwork, p.c.HubAddr)
}

func (p *Proxy) writeAndStream(b []byte, c io.ReadWriteCloser, i int) error {
	h, err := p.dialHub()

	if err != nil {
		return err
	}

	defer h.Close()

	if p.c.NoSendIP {
		b = b[i:]
	}

	if len(b) > 0 {
		_, err = h.Write(b)

		if err != nil {
			return err
		}
	}

	return p.stream(c, h)
}

func (p *Proxy) stream(c, h io.ReadWriteCloser) error {
	closeBoth := func() {
		_ = c.Close()
		_ = h.Close()
	}

	defer closeBoth()

	go func() {
		defer closeBoth()
		_, _ = p.copyBuffer(h, c, metrics.ConnRx)
	}()

	_, _ = p.copyBuffer(c, h, metrics.ConnTx)
	return nil
}

// copyBuffer was copied from io package and modified to add instrumentation

func (p *Proxy) copyBuffer(dst io.Writer, src io.Reader, cnt metrics.Counter) (written int64, err error) {
	size := p.c.Buffer * 1024

	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}

	buf := make([]byte, size)

	for {
		nr, er := src.Read(buf)

		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			cnt.Add(float64(nw))

			if nw > 0 {
				written += int64(nw)
			}

			if ew != nil {
				err = ew
				break
			}

			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}

		if er != nil {
			if er != io.EOF {
				err = er
			}

			break
		}
	}

	return written, err
}