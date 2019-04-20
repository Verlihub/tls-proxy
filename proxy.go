/*
	Copyright (C) 2019 Dexo, dexo at verlihub dot net
	Copyright (C) 2019 Verlihub Team, info at verlihub dot net

	This is free software; You can redistribute it
	and modify it under the terms of the GNU General
	Public License as published by the Free Software
	Foundation, either version 3 of the license, or at
	your option any later version.

	It is distributed in the hope that it will be
	useful, but without any warranty, without even the
	implied warranty of merchantability or fitness for
	a particular purpose. See the GNU General Public
	License for more details.

	Please see http://www.gnu.org/licenses/ for a copy
	of the GNU General Public License.
*/

// TLS Proxy 0.0.1.6

package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/Verlihub/tls-proxy/metrics"
)

var (
	fHost = flag.String("host", ":411", "Comma-separated list of hosts to listen on")
	fWait = flag.Duration("wait", 650*time.Millisecond, "Time to wait to detect the protocol")
	fHub = flag.String("hub", "127.0.0.1:411", "Hub address to connect to")
	fIP = flag.Bool("ip", true, "Send client IP")
	fLog = flag.Bool("log", false, "Enable connection logging")
	fCert = flag.String("cert", "hub.cert", "TLS .cert file")
	fKey = flag.String("key", "hub.key", "TLS .key file")
	fPProf = flag.String("pprof", "", "Serve profiler on a given address (empty = disabled)")
	fMetrics = flag.String("metrics", "", "Serve metrics on a given address (empty = disabled)")
	fBuf = flag.Int("buf", 10, "Buffer size in KB")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

var tlsConfig *tls.Config

func run() error {
	if *fCert != "" && *fKey != "" {
		log.Println("using certs:", *fCert, *fKey)
		var err error
		tlsConfig = &tls.Config{NextProtos: []string{"nmdc"}}
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(*fCert, *fKey)

		if err != nil {
			return err
		}

	} else {
		log.Println("no certs; TLS disabled")
	}

	if *fPProf != "" {
		log.Println("enabling profiler on", *fPProf)

		go func() {
			if err := http.ListenAndServe(*fPProf, nil); err != nil {
				log.Println("cannot enable profiler:", err)
			}
		}()
	}

	if *fMetrics != "" {
		log.Println("serving metrics on", *fMetrics)

		go func() {
			if err := metrics.ListenAndServe(*fMetrics); err != nil {
				log.Println("cannot serve metrics:", err)
			}
		}()
	}

	hosts := strings.Split(*fHost, ",")
	var lis []net.Listener

	defer func() {
		for _, l := range lis {
			_ = l.Close()
		}
	}()

	for _, host := range hosts {
		l, err := net.Listen("tcp4", host)

		if err != nil {
			return err
		}

		lis = append(lis, l)
	}

	var wg sync.WaitGroup

	for i, l := range lis {
		wg.Add(1)
		l := l
		log.Println("proxying", hosts[i], "to", *fHub)

		go func() {
			defer wg.Done()
			acceptOn(l)
		}()
	}

	wg.Wait()
	return nil
}

func acceptOn(l net.Listener) {
	for {
		c, err := l.Accept()

		if err != nil {
			if *fLog {
				log.Println(err)
			}

			metrics.ConnError.Add(1)
			continue
		}

		metrics.ConnAccepted.Add(1)

		go func() {
			err := serve(c)

			if err != nil && err != io.EOF {
				metrics.ConnError.Add(1)

				if *fLog {
					log.Println(c.RemoteAddr(), err)
				}
			}
		}()
	}
}

type timeoutErr interface {
	Timeout() bool
}

func serve(c net.Conn) error {
	metrics.ConnOpen.Add(1)

	defer func() {
		_ = c.Close()
		metrics.ConnOpen.Add(-1)
	}()

	buf := make([]byte, 1024)
	i := copy(buf, "$MyIP ")
	i += copy(buf[i:], c.RemoteAddr().(*net.TCPAddr).IP.String())
	i += copy(buf[i:], " 0.0|")

	if tlsConfig == nil || *fWait <= 0 { // no auto detection
		metrics.ConnInsecure.Add(1)
		metrics.ConnOpenInsecure.Add(1)
		defer metrics.ConnOpenInsecure.Add(-1)
		return writeAndStream(buf[:i], c, i)
	}

	err := c.SetReadDeadline(time.Now().Add(*fWait))

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
		return writeAndStream(buf[:i], c, i)
	}

	if err != nil {
		return err
	}

	buf = buf[:i+n]

	if n >= 2 && buf[i] == 0x16 && buf[i+1] == 0x03 {
		tlsBuf := buf[i:]
		c = &multiReadConn{r: io.MultiReader(bytes.NewReader(tlsBuf), c), Conn: c}
		tc := tls.Server(c, tlsConfig) // tls handshake
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
			default:
				buf[i-2] = '0'
		}

		buf = buf[:i]
		dt := time.Since(start).Seconds()
		metrics.ConnTLS.Add(1)
		metrics.ConnOpenTLS.Add(1)
		defer metrics.ConnOpenTLS.Add(-1)
		metrics.ConnTLSHandshake.Observe(dt)
		return writeAndStream(buf, tc, i)
	}

	metrics.ConnInsecure.Add(1)
	metrics.ConnOpenInsecure.Add(1)
	defer metrics.ConnOpenInsecure.Add(-1)
	return writeAndStream(buf, c, i)
}

type multiReadConn struct {
	r io.Reader
	net.Conn
}

func (c *multiReadConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func dialHub() (net.Conn, error) {
	return net.Dial("tcp4", *fHub)
}

func writeAndStream(p []byte, c io.ReadWriteCloser, i int) error {
	h, err := dialHub()

	if err != nil {
		return err
	}

	defer h.Close()

	if !*fIP {
		p = p[i:]
	}

	if len(p) > 0 {
		_, err = h.Write(p)

		if err != nil {
			return err
		}
	}

	return stream(c, h)
}

func stream(c, h io.ReadWriteCloser) error {
	closeBoth := func() {
		_ = c.Close()
		_ = h.Close()
	}

	defer closeBoth()

	go func() {
		defer closeBoth()
		_, _ = copyBuffer(h, c, metrics.ConnRx)
	}()

	_, _ = copyBuffer(c, h, metrics.ConnTx)
	return nil
}

// copyBuffer was copied from io package and modified to add instrumentation

func copyBuffer(dst io.Writer, src io.Reader, cnt metrics.Counter) (written int64, err error) {
	size := *fBuf * 1024

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

// end of file