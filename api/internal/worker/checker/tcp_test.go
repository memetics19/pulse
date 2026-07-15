package checker_test

import (
	"context"
	"net"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestTCPChecker_Up(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	c := checker.NewTCP(true)
	result := c.Check(context.Background(), ln.Addr().String(), 5)
	assert.Equal(t, "up", result.Status)
}

func TestTCPChecker_Down(t *testing.T) {
	c := checker.NewTCP(true)
	result := c.Check(context.Background(), "127.0.0.1:19998", 1)
	assert.Equal(t, "down", result.Status)
}
