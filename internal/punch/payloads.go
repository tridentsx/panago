package punch

import (
	"fmt"
	"net"
)

// Payload types
const (
	PayloadTypeShell  = "shell"
	PayloadTypeBackup = "backup"
)

// sendPayload sends a payload to the target IP
func sendPayload(ip string, port int, payload string) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(payload))
	return err
}

// IsAvailable checks if the punch service is available on the target IP
func IsAvailable(ip string, cfg *PunchConfig) bool {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Check initial port (60030)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, cfg.Port), cfg.Timeout)
	if err != nil {
		return false
	}
	conn.Close()

	// Check punch port (2222)
	conn, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, cfg.TargetPort), cfg.Timeout)
	if err != nil {
		return false
	}
	conn.Close()

	return true
}

// Activate sends the initial payload to activate the punch binary
func Activate(ip string, payloadType string, cfg *PunchConfig) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var payload string
	switch payloadType {
	case PayloadTypeShell:
		payload = fmt.Sprintf("SHELL:%s", ip)
	case PayloadTypeBackup:
		payload = fmt.Sprintf("BACKUP:%s", ip)
	default:
		return fmt.Errorf("unknown payload type: %s", payloadType)
	}

	// Send initial payload to activate punch
	if err := sendPayload(ip, cfg.Port, payload); err != nil {
		return fmt.Errorf("failed to send activation payload: %w", err)
	}

	// Send trigger payload to punch port
	if err := sendPayload(ip, cfg.TargetPort, "EXECUTE"); err != nil {
		return fmt.Errorf("failed to send trigger payload: %w", err)
	}

	return nil
}
