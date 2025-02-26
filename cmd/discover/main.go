package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/koron/go-ssdp"
)

// UPnP Device Description XML Struct
type DeviceDescription struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
	} `xml:"device"`
}

// Fetch and parse SSDP device XML description
func fetchDeviceInfo(url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching device info:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response:", err)
		return
	}

	var desc DeviceDescription
	if err := xml.Unmarshal(body, &desc); err != nil {
		log.Println("Error parsing XML:", err)
		return
	}

	fmt.Println("Device Found:")
	fmt.Println("  Name:         ", desc.Device.FriendlyName)
	fmt.Println("  Manufacturer: ", desc.Device.Manufacturer)
	fmt.Println("  Model:        ", desc.Device.ModelName)
	fmt.Println("-------------------------------------------------")
}

func main() {
	fmt.Println("Searching for SSDP devices...")

	// Send SSDP search request for all devices
	results, err := ssdp.Search(ssdp.All, 3, "")
	if err != nil {
		log.Fatal("Error during SSDP search:", err)
	}

	// Process found devices
	for _, service := range results {
		fmt.Println("Discovered SSDP Device:")
		fmt.Println("  Service Type:", service.Type)
		fmt.Println("  USN:         ", service.USN)
		fmt.Println("  Location:   ", service.Location)
		fmt.Println("-------------------------------------------------")

		// Fetch and parse the device description XML
		fetchDeviceInfo(service.Location)
	}
}
