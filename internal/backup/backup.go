package backup

import (
	"fmt"
	"io"
	"net"
	"time"
)

// Backup handles backup operations
type Backup struct {
	conn net.Conn
}

// New creates a new backup session
func New(ip string) (*Backup, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:2222", ip), time.Second*2)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &Backup{conn: conn}, nil
}

func (b *Backup) CreateBackup() error {
	// Send backup command
	if _, err := b.conn.Write([]byte("BACKUP\n")); err != nil {
		return fmt.Errorf("failed to send backup command: %w", err)
	}

	// Read response
	buf := make([]byte, 1024)
	n, err := b.conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if string(buf[:n]) != "OK\n" {
		return fmt.Errorf("invalid response: %s", string(buf[:n]))
	}

	return nil
}

func (b *Backup) Close() error {
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// Rest of backup.go implementation...
