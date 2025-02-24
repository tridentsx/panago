//go:build windows
// +build windows

// This file contains Windows-specific implementations
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)



func (w WindowsDiskManager) ListUSBDisks() ([]Disk, error) {
	cmd := exec.Command("powershell", "-Command",
		`Get-PhysicalDisk | Where-Object MediaType -eq 'Removable' | Select-Object DeviceId, Model, Size | ConvertTo-Json`)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var disks []Disk
	if err := json.Unmarshal(out.Bytes(), &disks); err != nil {
		return nil, err
	}

	for i := range disks {
		disks[i].DevicePath = fmt.Sprintf(`\\.\PhysicalDrive%d`, i)
	}

	return disks, nil
}

func (w WindowsDiskManager) FormatDisk(disk Disk) error {
	fmt.Println("Formatting drive using DiskPart...")
	script := fmt.Sprintf("select disk %s\nclean\ncreate partition primary\nformat fs=NTFS quick\nassign\nexit", disk.Name)
	cmd := exec.Command("diskpart")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w WindowsDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	fmt.Printf("Extracting %s to %s...\n", tarFile, disk.DevicePath)
	cmd := exec.Command("tar", "-xvf", tarFile, "-C", disk.DevicePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
