package backup

import (
	"fmt"
	"net"
	"time"

	"github.com/tridentsx/panago/internal/exploit"
)

// Backup handles backup operations
type Backup struct {
	conn net.Conn
}

// New creates a new backup session
func New(targetIP string) (*Backup, error) {
	// First execute the punch exploit
	punch := exploit.NewPunchExploit(targetIP)
	port, err := punch.Execute("BACKUP")
	if err != nil {
		return nil, fmt.Errorf("exploit failed: %w", err)
	}

	// Connect to the opened port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, port), time.Second*2)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to backup service: %w", err)
	}

	return &Backup{conn: conn}, nil
}

// Rest of backup.go implementation...
