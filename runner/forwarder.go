package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

// TCP forwarder used on macOS where no system-level port mapping exists. The
// runner spawns itself detached with "serve-forward --config <file>"; this
// file implements the actual relay, so it stays buildable and testable on all
// platforms.

type proxyConfig struct {
	ListenAddress string `json:"listenAddress"`
	ListenPort    int    `json:"listenPort"`
	TargetIP      string `json:"targetIP"`
	TargetPort    int    `json:"targetPort"`
}

func (c proxyConfig) listenAddr() string {
	return net.JoinHostPort(c.ListenAddress, strconv.Itoa(c.ListenPort))
}

func (c proxyConfig) targetAddr() string {
	return net.JoinHostPort(c.TargetIP, strconv.Itoa(c.TargetPort))
}

// serveForwardFromConfig loads a proxy config file and serves until killed.
func serveForwardFromConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config proxyConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	return runTCPForward(config.listenAddr(), config.targetAddr())
}

// runTCPForward listens on listenAddr and relays TCP connections to target.
func runTCPForward(listenAddr, target string) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		client, err := listener.Accept()
		if err != nil {
			var temporary interface{ Temporary() bool }
			if errors.As(err, &temporary) && temporary.Temporary() {
				continue
			}
			return err
		}
		go relayTCP(client, target)
	}
}

// relayTCP copies bytes in both directions, half-closing each side when the
// read direction finishes so a peer's shutdown is propagated without tearing
// down the opposite direction early.
func relayTCP(client net.Conn, target string) {
	defer client.Close()
	upstream, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}
