//go:build linux

package disk

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// LinuxDiskManager implements DiskManager for Linux systems
type LinuxDiskManager struct{}

func newPlatformManager() DiskManager {
	return LinuxDiskManager{}
}

func (l LinuxDiskManager) ListUSBDisks() ([]Disk, error) {
	cmd := exec.Command("lsblk", "-Jbo", "NAME,MODEL,SIZE,TRAN,TYPE")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %v", err)
	}

	var result struct {
		BlockDevices []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
			Size  int64  `json:"size"`
			Tran  string `json:"tran"`
			Type  string `json:"type"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse disk info: %v", err)
	}

	var disks []Disk
	for _, d := range result.BlockDevices {
		if d.Tran == "usb" && d.Type == "disk" {
			disks = append(disks, Disk{
				Name:       d.Name,
				Model:      d.Model,
				Size:       FormatSize(d.Size),
				DevicePath: "/dev/" + d.Name,
			})
		}
	}
	return disks, nil
}

func (l LinuxDiskManager) Format(disk Disk) error {
	// Unmount all partitions first
	exec.Command("sh", "-c", fmt.Sprintf("umount %s* 2>/dev/null || true", disk.DevicePath)).Run()
	return exec.Command("mkfs.ext4", "-F", disk.DevicePath).Run()
}

func (l LinuxDiskManager) WriteImage(disk Disk, imageFile string, progress func(string)) error {
	return ExtractImageWithProgress(disk, imageFile, progress)
}
