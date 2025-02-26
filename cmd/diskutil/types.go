package main

// Disk represents a physical disk device
type Disk struct {
	Name       string
	Model      string
	Size       string
	DevicePath string
	Mounted    bool
	Number     int // For Windows disk number
}

// DiskManager interface defines disk operations
type DiskManager interface {
	ListUSBDisks() ([]Disk, error)
	Format(disk Disk) error
	WriteImage(disk Disk, imageFile string, progress func(string)) error
}

// WindowsDiskManager implements DiskManager for Windows systems
type WindowsDiskManager struct{}

// LinuxDiskManager implements DiskManager for Linux systems
type LinuxDiskManager struct{}

// MacOSDiskManager implements DiskManager for macOS systems
type MacOSDiskManager struct{}
