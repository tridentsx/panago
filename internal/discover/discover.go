package discover

import (
	"fmt"
	"net"
	"time"
)

type PlayerInfo struct {
	Version string
	Model   string
}

// CheckPlayer attempts to detect a player at the given IP
func CheckPlayer(ip string) (*PlayerInfo, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:60030", ip), time.Second*2)
	if err != nil {
		return nil, fmt.Errorf("player not detected: %w", err)
	}
	defer conn.Close()

	// For now, return dummy info
	return &PlayerInfo{
		Version: "1.0",
		Model:   "Test Player",
	}, nil
}
