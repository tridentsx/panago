package punch

import "time"

// Option is a function that configures a PunchSession
type Option func(*PunchSession)

// WithInitialPort sets a custom initial port
func WithInitialPort(port int) Option {
	return func(s *PunchSession) {
		s.initialPort = port
	}
}

// WithPunchPort sets a custom punch port
func WithPunchPort(port int) Option {
	return func(s *PunchSession) {
		s.punchPort = port
	}
}

// WithTimeout sets a custom timeout
func WithTimeout(timeout time.Duration) Option {
	return func(s *PunchSession) {
		s.timeout = timeout
	}
}
