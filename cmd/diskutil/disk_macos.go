//go:build darwin
// +build darwin

// This file contains macOS-specific implementations
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func (m MacOSDiskManager) ListUSBDisks() ([]Disk, error) {
	cmd := exec.Command("diskutil", "list", "-plist")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var result struct {
		AllDisksAndPartitions []struct {
			DeviceIdentifier string `json:"DeviceIdentifier"`
			Content          string `json:"Content,omitempty"`
			Size             int64  `json:"Size,omitempty"`
			MountPoint       string `json:"MountPoint,omitempty"`
		} `json:"AllDisksAndPartitions"`
	}

	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, err
	}

	var disks []Disk
	for _, d := range result.AllDisksAndPartitions {
		if strings.HasPrefix(d.DeviceIdentifier, "disk") && d.Content == "" { // USB disks often have empty content before formatting
			disks = append(disks, Disk{
				Name:       d.DeviceIdentifier,
				Model:      "USB Drive",
				Size:       fmt.Sprintf("%d GB", d.Size/(1024*1024*1024)),
				DevicePath: "/dev/" + d.DeviceIdentifier,
			})
		}
	}
	return disks, nil
}

func (m MacOSDiskManager) FormatDisk(disk Disk) error {
	fmt.Printf("Preparing %s for disk image writing...\n", disk.DevicePath)

	// Unmount all volumes on this disk
	cmd := exec.Command("diskutil", "unmountDisk", disk.DevicePath)
	return cmd.Run()
}

func (m MacOSDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	return fmt.Errorf("MountAndExtract not used for raw disk images")
}
