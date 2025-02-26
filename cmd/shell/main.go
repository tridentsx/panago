package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/tridentsx/panago/internal/shell"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: shell <target-ip>")
		os.Exit(1)
	}

	targetIP := os.Args[1]

	// Create new shell session
	sh, err := shell.New(targetIP)
	if err != nil {
		log.Fatal("Failed to create shell session:", err)
	}
	defer sh.Close()

	// Execute command
	if err := sh.ExecuteCommand("ls"); err != nil {
		log.Fatal("Failed to execute command:", err)
	}

	// Get output
	output, err := sh.GetOutput()
	if err != nil {
		log.Fatal("Failed to get output:", err)
	}

	// Copy output to stdout
	if _, err := io.Copy(os.Stdout, output); err != nil {
		log.Fatal("Failed to read output:", err)
	}
}
