package main

// DiskManager defines the operations that must be implemented by OS-specific managers
type DiskManager interface {
	ListUSBDisks() ([]Disk, error)                   // Lists USB disks
	FormatDisk(disk Disk) error                      // Formats the selected disk
	MountAndExtract(disk Disk, tarFile string) error // Mounts and extracts tar archive
}

// Disk represents a detected USB disk
type Disk struct {
	Name       string
	Model      string
	Size       string
	DevicePath string
}

// WindowsDiskManager implements DiskManager for Windows systems
type WindowsDiskManager struct{}

// LinuxDiskManager implements DiskManager for Linux systems
type LinuxDiskManager struct{}

// MacOSDiskManager implements DiskManager for macOS systems
type MacOSDiskManager struct{}