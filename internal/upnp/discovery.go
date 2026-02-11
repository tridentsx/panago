package upnp

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/koron/go-ssdp"
)

// DeviceInfo represents a discovered UPnP device
type DeviceInfo struct {
	Location        string
	ServiceType     string
	USN             string
	FriendlyName    string
	Manufacturer    string
	ModelName       string
	DeviceType      string
	PresentationURL string
}

// Discoverer handles UPnP device discovery
type Discoverer struct {
	timeout time.Duration
	client  *http.Client
}

// NewDiscoverer creates a new UPnP discoverer
func NewDiscoverer(timeout int) *Discoverer {
	return &Discoverer{
		timeout: time.Duration(timeout) * time.Second,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Discover searches for UPnP devices
func (d *Discoverer) Discover() ([]DeviceInfo, error) {
	services, err := ssdp.Search(ssdp.All, int(d.timeout.Seconds()), "")
	if err != nil {
		return nil, fmt.Errorf("SSDP search failed: %w", err)
	}

	var devices []DeviceInfo
	for _, service := range services {
		device, err := d.getDeviceInfo(service)
		if err != nil {
			continue
		}
		devices = append(devices, device)
	}

	return devices, nil
}

func (d *Discoverer) getDeviceInfo(service ssdp.Service) (DeviceInfo, error) {
	device := DeviceInfo{
		Location:    service.Location,
		ServiceType: service.Type,
		USN:         service.USN,
	}

	resp, err := d.client.Get(service.Location)
	if err != nil {
		return device, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return device, err
	}

	var desc struct {
		Device struct {
			FriendlyName    string `xml:"friendlyName"`
			Manufacturer    string `xml:"manufacturer"`
			ModelName       string `xml:"modelName"`
			DeviceType      string `xml:"deviceType"`
			PresentationURL string `xml:"presentationURL"`
		} `xml:"device"`
	}

	if err := xml.Unmarshal(body, &desc); err != nil {
		return device, err
	}

	device.FriendlyName = desc.Device.FriendlyName
	device.Manufacturer = desc.Device.Manufacturer
	device.ModelName = desc.Device.ModelName
	device.DeviceType = desc.Device.DeviceType
	device.PresentationURL = desc.Device.PresentationURL

	return device, nil
}

// ExtractIP parses a URL and returns just the host (without port).
func ExtractIP(location string) string {
	u, err := url.Parse(location)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	return host
}

// DiscoverPanasonic discovers UPnP devices and returns only those
// whose Manufacturer contains "Panasonic".
func DiscoverPanasonic(timeout int) ([]DeviceInfo, error) {
	d := NewDiscoverer(timeout)
	devices, err := d.Discover()
	if err != nil {
		return nil, err
	}

	var panasonic []DeviceInfo
	for _, dev := range devices {
		if strings.Contains(dev.Manufacturer, "Panasonic") {
			panasonic = append(panasonic, dev)
		}
	}
	return panasonic, nil
}
