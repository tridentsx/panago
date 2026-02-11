package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/internal/disk"
)

var diskCmd = &cobra.Command{
	Use:   "disk",
	Short: "USB disk management tools",
	Long:  "List USB disks, format, and write disk images.",
}

var diskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available USB disks",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := disk.NewDiskManager()
		disks, err := mgr.ListUSBDisks()
		if err != nil {
			return err
		}
		if len(disks) == 0 {
			fmt.Println("No USB disks found.")
			return nil
		}
		for _, d := range disks {
			fmt.Printf("%s (%s) - %s [%s]\n", d.Name, d.Model, d.Size, d.DevicePath)
		}
		return nil
	},
}

var diskWriteCmd = &cobra.Command{
	Use:   "write <device> <image.gz>",
	Short: "Write disk image to a USB device",
	Long:  "Formats the target device and writes a (possibly gzipped) disk image to it.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		devicePath := args[0]
		imageFile := args[1]

		d := disk.Disk{DevicePath: devicePath}
		mgr := disk.NewDiskManager()

		fmt.Printf("Formatting %s...\n", devicePath)
		if err := mgr.Format(d); err != nil {
			return fmt.Errorf("format failed: %w", err)
		}

		fmt.Printf("Writing %s to %s...\n", imageFile, devicePath)
		progress := func(msg string) {
			fmt.Printf("\r%s", msg)
		}
		if err := mgr.WriteImage(d, imageFile, progress); err != nil {
			return fmt.Errorf("write failed: %w", err)
		}

		fmt.Println("\nDone.")
		return nil
	},
}

func init() {
	diskCmd.AddCommand(diskListCmd)
	diskCmd.AddCommand(diskWriteCmd)
	rootCmd.AddCommand(diskCmd)
}
