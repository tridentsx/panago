package shell

import (
	"net"
	"testing"
)

func TestShellSocket(t *testing.T) {
	// Add tests
}

func setupTestServer(t *testing.T) net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
	return l
}
