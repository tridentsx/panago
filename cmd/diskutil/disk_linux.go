//go:build linux
// +build linux

// This file contains Linux-specific implementations
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

func (l LinuxDiskManager) ListUSBDisks() ([]Disk, error) {
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,MODEL,SIZE,TRAN,TYPE")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var result struct {
		BlockDevices []struct {
			Name      string `json:"name"`
			Model     string `json:"model"`
			Size      string `json:"size"`
			Transport string `json:"tran"`
			Type      string `json:"type"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, err
	}

	var disks []Disk
	for _, d := range result.BlockDevices {
		if d.Transport == "usb" && d.Type == "disk" {
			disks = append(disks, Disk{
				Name:       d.Name,
				Model:      d.Model,
				Size:       d.Size,
				DevicePath: "/dev/" + d.Name,
			})
		}
	}
	return disks, nil
}

func (l LinuxDiskManager) FormatDisk(disk Disk) error {
	// For raw disk images, we don't need to format since the image contains partitions
	fmt.Printf("Preparing %s for disk image writing...\n", disk.DevicePath)

	// We should unmount any mounted partitions from this disk
	cmd := exec.Command("sh", "-c", fmt.Sprintf("umount %s* 2>/dev/null || true", disk.DevicePath))
	cmd.Run() // Ignore errors as partitions might not be mounted

	return nil
}

func (l LinuxDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	return fmt.Errorf("MountAndExtract not used for raw disk images")
}
