package disk

import "fmt"

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

// FormatSize converts bytes to human-readable size
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
