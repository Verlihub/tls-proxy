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

// TLS Proxy 0.0.2.0

package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/Verlihub/tls-proxy/dcproxy"
)

var (
	fHost    = flag.String("host", ":411", "Comma-separated list of hosts to listen on")
	fWait    = flag.Duration("wait", 650*time.Millisecond, "Time to wait to detect the protocol")
	fHub     = flag.String("hub", "127.0.0.1:411", "Hub address to connect to")
	fHubNet  = flag.String("net", "tcp4", "Hub network (tcp4, tcp6, tcp, unix)")
	fIP      = flag.Bool("ip", true, "Send client IP")
	fLog     = flag.Bool("log", false, "Enable connection logging")
	fCert    = flag.String("cert", "hub.cert", "TLS .cert file")
	fKey     = flag.String("key", "hub.key", "TLS .key file")
	fPProf   = flag.String("pprof", "", "Serve profiler on a given address (empty = disabled)")
	fMetrics = flag.String("metrics", "", "Serve metrics on a given address (empty = disabled)")
	fBuf     = flag.Int("buf", 10, "Buffer size in KB")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	p, err := dcproxy.New(dcproxy.Config{
		HubAddr:    *fHub,
		HubNetwork: *fHubNet,
		Hosts:      strings.Split(*fHost, ","),
		Cert:       *fCert,
		Key:        *fKey,
		PProf:      *fPProf,
		Metrics:    *fMetrics,
		LogErrors:  *fLog,
		Wait:       *fWait,
		Buffer:     *fBuf,
		NoSendIP:   !*fIP,
	})
	if err != nil {
		return err
	}
	err = p.Run()
	if err != nil {
		return err
	}
	defer p.Close()

	p.Wait()
	return nil
}
