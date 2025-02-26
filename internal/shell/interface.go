package shell

import "io"

type Shell interface {
	Connect(target string) error
	ExecuteCommand(cmd string) error
	GetOutput() (io.Reader, error)
	Close() error
}
