//go:build darwin
// +build darwin

// This file contains macOS-specific implementations
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	fmt.Printf("Formatting %s as exFAT...\n", disk.DevicePath)
	cmd := exec.Command("diskutil", "eraseDisk", "exFAT", "USB_DRIVE", disk.DevicePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m MacOSDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	fmt.Printf("Mounting %s...\n", disk.DevicePath)
	if err := exec.Command("diskutil", "mountDisk", disk.DevicePath).Run(); err != nil {
		return err
	}

	// Find the mount point
	mountPointCmd := exec.Command("diskutil", "info", disk.DevicePath)
	var out bytes.Buffer
	mountPointCmd.Stdout = &out
	if err := mountPointCmd.Run(); err != nil {
		return err
	}

	mountPoint := ""
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "Mount Point:") {
			mountPoint = strings.TrimSpace(strings.Split(line, ":")[1])
			break
		}
	}

	if mountPoint == "" {
		return fmt.Errorf("could not determine mount point")
	}

	fmt.Printf("Extracting %s to %s...\n", tarFile, mountPoint)
	cmd := exec.Command("tar", "-xvf", tarFile, "-C", mountPoint)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
