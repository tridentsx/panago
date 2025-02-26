package punch

import (
	"fmt"
	"io"
	"net"
	"time"
)

const (
	// Default ports and timeouts
	defaultInitialPort = 60030
	defaultPunchPort   = 2222
	defaultTimeout     = 2 * time.Second

	// Operation types
	OpTypeShell  = "SHELL"
	OpTypeBackup = "BACKUP"
	OpTypeUpdate = "UPDATE"
)

// PunchSession represents an active session with a target device
type PunchSession struct {
	targetIP    string
	initialPort int
	punchPort   int
	timeout     time.Duration
	conn        net.Conn
}

// NewSession creates a new punch session with the target
func NewSession(targetIP string, opts ...Option) *PunchSession {
	s := &PunchSession{
		targetIP:    targetIP,
		initialPort: defaultInitialPort,
		punchPort:   defaultPunchPort,
		timeout:     defaultTimeout,
	}

	// Apply any custom options
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Connect establishes the initial connection and triggers the punch
func (s *PunchSession) Connect() error {
	// Check if service is available
	if !s.isAvailable() {
		return fmt.Errorf("punch service not available on %s", s.targetIP)
	}

	// Connect to initial port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", s.targetIP, s.initialPort), s.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to initial port: %w", err)
	}
	s.conn = conn

	return nil
}

// StartOperation initiates the specified operation type
func (s *PunchSession) StartOperation(opType string) error {
	if s.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Send operation trigger
	payload := fmt.Sprintf("%s:%s", opType, s.targetIP)
	if _, err := s.conn.Write([]byte(payload)); err != nil {
		return fmt.Errorf("failed to send operation trigger: %w", err)
	}

	// Connect to punch port
	punchConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", s.targetIP, s.punchPort), s.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to punch port: %w", err)
	}

	// Close initial connection and use punch connection
	s.conn.Close()
	s.conn = punchConn

	return nil
}

// ExecuteCommand sends a command to the punch port
func (s *PunchSession) ExecuteCommand(cmd string) error {
	if s.conn == nil {
		return fmt.Errorf("not connected")
	}

	_, err := s.conn.Write([]byte(cmd + "\n"))
	return err
}

// GetResponse reads the response from the connection
func (s *PunchSession) GetResponse() (io.Reader, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("not connected")
	}
	return s.conn, nil
}

// Close closes the session
func (s *PunchSession) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// isAvailable checks if the punch service is available
func (s *PunchSession) isAvailable() bool {
	// Try initial port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", s.targetIP, s.initialPort), s.timeout)
	if err != nil {
		return false
	}
	conn.Close()

	// Try punch port
	conn, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", s.targetIP, s.punchPort), s.timeout)
	if err != nil {
		return false
	}
	conn.Close()

	return true
}
