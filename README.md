# TLS proxy

TLS proxy server for NMDC protocol. Currently supported by Verlihub 1.2.0.5 and later.

## Generate self signed certificate

`openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out "hub.crt" -keyout "hub.key"`

## Install Go

`sudo apt install golang`

## Compile proxy server

```
git clone https://github.com/verlihub/tls-proxy.git
cd tls-proxy
export CGO_ENABLED=0 && go build -ldflags "-libgcc=none" -tags netgo proxy.go
```

## Start proxy server

`./proxy --cert="/path/to/hub.crt" --key="/path/to/hub.key" --host="1.2.3.4:411" --hub="127.0.0.1:411"`

`1.2.3.4:411` is the proxy listening socket, the address that hub would normally be listening on. `127.0.0.1:411` is the hub listening socket, the address that accepts connections from the proxy. Add `&` at the end of command to run the process in background.

## Configure the hub

`!set listen_ip 127.0.0.1`

`!set tls_proxy_ip 127.0.0.1`

Then start the hub as usual.

## Protocol specification

Following command is sent to the hub right after the connection is established:

`$MyIP 2.3.4.5 1.0|`

`2.3.4.5` is the real IP address of connected user. Second parameter stands for TLS version used by client, in case of non secure connection, `0.0` is passed to hub.

## Important to know

Remember to set twice the regular `ulimit` before starting both servers, because there will be twice as many connections than you would usually have.
