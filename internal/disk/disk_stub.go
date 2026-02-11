//go:build !windows && !linux && !darwin

package disk

import "fmt"

// stubDiskManager is used on unsupported platforms
type stubDiskManager struct{}

func newPlatformManager() DiskManager {
	return stubDiskManager{}
}

func (s stubDiskManager) ListUSBDisks() ([]Disk, error) {
	return nil, fmt.Errorf("disk management not supported on this platform")
}

func (s stubDiskManager) Format(disk Disk) error {
	return fmt.Errorf("disk management not supported on this platform")
}

func (s stubDiskManager) WriteImage(disk Disk, imageFile string, progress func(string)) error {
	return fmt.Errorf("disk management not supported on this platform")
}
