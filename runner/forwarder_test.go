package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

// freePort returns a currently-free TCP port on 127.0.0.1. Environments that
// forbid binding sockets at all (sandboxes, restricted containers) skip the
// network tests instead of failing them.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == syscall.EPERM {
			t.Skip("当前环境不允许绑定本地端口，跳过网络转发测试")
		}
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("当前环境不允许绑定本地端口，跳过网络转发测试")
		}
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestRunTCPForwardRelaysTraffic(t *testing.T) {
	echoAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	echo, err := net.Listen("tcp", echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			client, acceptErr := echo.Accept()
			if acceptErr != nil {
				return
			}
			go func(connection net.Conn) {
				defer connection.Close()
				_, _ = io.Copy(connection, connection)
			}(client)
		}
	}()

	listenAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	go func() { _ = runTCPForward(listenAddr, echoAddr) }()

	// The listener starts asynchronously; retry the dial briefly.
	upstream := dialWithRetry(t, listenAddr)
	defer upstream.Close()

	payload := "alex ping via forwarder"
	if _, err := fmt.Fprintln(upstream, payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	reader := bufio.NewReader(upstream)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if got := strings.TrimSpace(line); got != payload {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}
}

func TestRunTCPForwardRejectsUnroutableTargetGracefully(t *testing.T) {
	// A dead target must not crash the forwarder listener; the client just
	// gets dropped. The listener keeps running: a second connection after a
	// failed relay must still succeed.
	listenAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	go func() { _ = runTCPForward(listenAddr, "127.0.0.1:1") }()

	// First connection triggers a relay to the unreachable target, which fails
	// inside relayTCP without panicking. Closing it drops that relay.
	first := dialWithRetry(t, listenAddr)
	_, _ = first.Write([]byte("x"))
	_ = first.Close()

	// Give relayTCP a moment to finish tearing down, then prove the listener
	// is still accepting new connections.
	time.Sleep(200 * time.Millisecond)
	second := dialWithRetry(t, listenAddr)
	_ = second.Close()
}

func dialWithRetry(t *testing.T, listenAddr string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		connection, dialErr := net.Dial("tcp", listenAddr)
		if dialErr == nil {
			return connection
		}
		if time.Now().After(deadline) {
			t.Fatalf("forward listener never came up: %v", dialErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
