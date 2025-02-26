package errors

import "fmt"

type ShellError struct {
	Op  string
	Err error
}

func (e *ShellError) Error() string {
	return fmt.Sprintf("shell operation %s failed: %v", e.Op, e.Err)
}

// Add other error types as needed
