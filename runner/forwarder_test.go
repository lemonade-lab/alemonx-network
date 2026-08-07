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
	done := make(chan error, 1)
	go func() { done <- runTCPForward(listenAddr, echoAddr) }()

	// The listener starts asynchronously; retry the dial briefly.
	var upstream net.Conn
	deadline := time.Now().Add(3 * time.Second)
	for {
		upstream, err = net.Dial("tcp", listenAddr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("forward listener never came up: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
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
	// gets dropped. This verifies relayTCP's error path stays contained.
	listenAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	done := make(chan error, 1)
	go func() { done <- runTCPForward(listenAddr, "127.0.0.1:1") }()

	deadline := time.Now().Add(3 * time.Second)
	var upstream net.Conn
	for {
		connection, dialErr := net.Dial("tcp", listenAddr)
		if dialErr == nil {
			upstream = connection
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("forward listener never came up: %v", dialErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Dialing succeeded and relayTCP must return without panicking.
	_, _ = upstream.Write([]byte("x"))
	_ = upstream.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("forwarder listener exited unexpectedly: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forwarder listener should keep accepting after a failed relay")
	}
}
