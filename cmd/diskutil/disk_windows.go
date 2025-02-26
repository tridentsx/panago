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
	fmt.Printf("Preparing %s for disk image writing...\n", disk.DevicePath)

	// For Windows, we need to clean the disk but not format it
	script := fmt.Sprintf("select disk %s\nclean\nexit", disk.Name)
	cmd := exec.Command("diskpart")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w WindowsDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	return fmt.Errorf("MountAndExtract not used for raw disk images")
}
