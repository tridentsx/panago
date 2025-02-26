//go:build windows
// +build windows

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func (w WindowsDiskManager) ListUSBDisks() ([]Disk, error) {
	cmd := exec.Command("powershell", "-Command",
		`Get-Disk | Where-Object { $_.BusType -eq 'USB' } | Select-Object Number,FriendlyName,Size | ConvertTo-Json`)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %v", err)
	}

	var rawDisks []struct {
		Number       int    `json:"Number"`
		FriendlyName string `json:"FriendlyName"`
		Size         int64  `json:"Size"`
	}
	if err := json.Unmarshal(output, &rawDisks); err != nil {
		return nil, fmt.Errorf("failed to parse disk info: %v", err)
	}

	var disks []Disk
	for _, d := range rawDisks {
		disks = append(disks, Disk{
			Name:       fmt.Sprintf("Disk %d", d.Number),
			Model:      d.FriendlyName,
			Size:       formatSize(d.Size),
			DevicePath: fmt.Sprintf("\\\\.\\PhysicalDrive%d", d.Number),
			Number:     d.Number,
		})
	}
	return disks, nil
}

func (w WindowsDiskManager) Format(disk Disk) error {
	script := fmt.Sprintf("select disk %d\nclean\nexit", disk.Number)
	cmd := exec.Command("diskpart")
	cmd.Stdin = strings.NewReader(script)
	return cmd.Run()
}

func (w WindowsDiskManager) WriteImage(disk Disk, imageFile string, progress func(string)) error {
	return extractImageWindowsWithProgress(disk, imageFile, progress)
}
