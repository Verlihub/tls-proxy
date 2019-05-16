package main

/*
#include "dcproxy_types.h"
*/
import "C"

import (
	"strings"
	"time"

	"github.com/Verlihub/tls-proxy/dcproxy"
)

var lastErr error

func setLastErr(err error) {
	lastErr = err
}

//export DCLastError
func DCLastError() *C.char {
	if lastErr == nil {
		return nil
	}
	e := lastErr.Error()
	return C.CString(e)
}

var curProxy *dcproxy.Proxy

//export DCProxyStart
func DCProxyStart(conf *C.DCProxyConfig) C.int {
	c := dcproxy.Config{
		HubAddr:    C.GoString(conf.HubAddr),
		HubNetwork: C.GoString(conf.HubNetwork),
		Hosts:      strings.Split(C.GoString(conf.Hosts), ","),
		Cert:       C.GoString(conf.Cert),
		Key:        C.GoString(conf.Key),
		CertOrg:    C.GoString(conf.CertOrg),
		CertHost:   C.GoString(conf.CertHost),
		PProf:      C.GoString(conf.PProf),
		Metrics:    C.GoString(conf.Metrics),
		LogErrors:  bool(conf.LogErrors),
		Wait:       time.Duration(conf.Wait) * time.Millisecond,
		Buffer:     int(conf.Buffer),
		NoSendIP:   bool(conf.NoSendIP),
	}
	p, err := dcproxy.New(c)
	if err != nil {
		setLastErr(err)
		return 0
	}
	err = p.Run()
	if err != nil {
		setLastErr(err)
		return 0
	}
	curProxy = p
	return 1
}

//export DCProxyStop
func DCProxyStop() {
	if curProxy == nil {
		return
	}
	_ = curProxy.Close()
	curProxy = nil
}

func main() {}
