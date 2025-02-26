package punch

import (
	"time"
)

// PunchConfig holds configuration for punch operations
type PunchConfig struct {
	Port       int
	TargetPort int
	Timeout    time.Duration
}

// DefaultConfig returns the default punch configuration
func DefaultConfig() *PunchConfig {
	return &PunchConfig{
		Port:       60030, // Initial connection port
		TargetPort: 2222,  // Port where punch binary listens
		Timeout:    2 * time.Second,
	}
}
