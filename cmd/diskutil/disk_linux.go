//go:build linux
// +build linux

// This file contains Linux-specific implementations
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	fmt.Printf("Formatting %s as ext4...\n", disk.DevicePath)
	cmd := exec.Command("mkfs.ext4", "-F", disk.DevicePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (l LinuxDiskManager) MountAndExtract(disk Disk, tarFile string) error {
	mountPoint := "/mnt/newdisk"
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return err
	}

	fmt.Printf("Mounting %s at %s...\n", disk.DevicePath, mountPoint)
	if err := exec.Command("mount", disk.DevicePath, mountPoint).Run(); err != nil {
		return err
	}
	defer exec.Command("umount", mountPoint).Run()

	fmt.Printf("Extracting %s to %s...\n", tarFile, mountPoint)
	cmd := exec.Command("tar", "-xvf", tarFile, "-C", mountPoint)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
