/*
	Copyright (C) 2019 Dexo
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

package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"time"
)

var (
	fHost = flag.String("host", ":411", "Host to listen on")
	fWait = flag.Duration("wait", 650*time.Millisecond, "Time to wait to detect the protocol")
	fHub  = flag.String("hub", "127.0.0.1:411", "Hub address to connect to")
	fIP   = flag.Bool("ip", true, "Send client IP")
	fCert = flag.String("cert", "hub.cert", "TLS .cert file")
	fKey  = flag.String("key", "hub.key", "TLS .key file")
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

	l, err := net.Listen("tcp4", *fHost)
	if err != nil {
		return err
	}
	defer l.Close()

	log.Println("proxying", *fHost, "to", *fHub)
	for {
		c, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func() {
			err := serve(c)
			if err != nil && err != io.EOF {
				log.Println(c.RemoteAddr(), err)
			}
		}()
	}
}

type timeoutErr interface {
	Timeout() bool
}

func serve(c net.Conn) error {
	defer c.Close()

	addr := c.RemoteAddr().(*net.TCPAddr)
	ip := addr.IP.String()

	buf := make([]byte, 1024)
	i := copy(buf, "$MyIP ")
	i += copy(buf[i:], ip)
	i += copy(buf[i:], " P|")

	if tlsConfig == nil || *fWait <= 0 {
		// no auto-detection
		return writeAndStream(buf[:i], c, i)
	}

	err := c.SetReadDeadline(time.Now().Add(*fWait))
	if err != nil {
		return err
	}

	n, err := c.Read(buf[i:])
	_ = c.SetReadDeadline(time.Time{})
	if e, ok := err.(timeoutErr); ok && e.Timeout() {
		// has to be NMDC
		return writeAndStream(buf[:i], c, i)
	}
	if err != nil {
		return err
	}
	buf = buf[:i+n]
	if n >= 2 && buf[i] == 0x16 && buf[i+1] == 0x03 {
		tlsBuf := buf[i:]
		buf[i-2] = 'S'
		buf = buf[:i]
		c = &multiReadConn{r: io.MultiReader(bytes.NewReader(tlsBuf), c), Conn: c}
		// TLS handshake
		tc := tls.Server(c, tlsConfig)
		defer tc.Close()
		err = tc.Handshake()
		if err != nil {
			return err
		}
		return writeAndStream(buf, tc, i)
	}
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

func stream(c1, c2 io.ReadWriteCloser) error {
	closeBoth := func() {
		_ = c1.Close()
		_ = c2.Close()
	}
	defer closeBoth()

	go func() {
		defer closeBoth()
		_, _ = io.Copy(c2, c1)
	}()
	_, _ = io.Copy(c1, c2)
	return nil
}

// end of file
