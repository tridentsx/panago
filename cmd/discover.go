package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/internal/upnp"
)

var discoverPanasonic bool

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover UPnP devices on the network",
	Long:  "Uses SSDP to scan for UPnP devices. Use --panasonic to filter for Panasonic players only.",
	RunE:  runDiscover,
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverPanasonic, "panasonic", false, "Show only Panasonic players")
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	fmt.Println("Scanning for UPnP devices...")

	var devices []upnp.DeviceInfo
	var err error

	if discoverPanasonic {
		devices, err = upnp.DiscoverPanasonic(3)
	} else {
		d := upnp.NewDiscoverer(3)
		devices, err = d.Discover()
	}
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No devices found.")
		return nil
	}

	fmt.Printf("\nFound %d device(s):\n\n", len(devices))
	for _, dev := range devices {
		ip := upnp.ExtractIP(dev.Location)
		fmt.Printf("  %s (%s)\n", dev.FriendlyName, dev.Manufacturer)
		fmt.Printf("    Model: %s\n", dev.ModelName)
		fmt.Printf("    IP:    %s\n", ip)
		fmt.Println()
	}
	return nil
}
