package disk

// NewDiskManager returns the platform-specific disk manager.
// See disk_linux.go, disk_macos.go, disk_windows.go, disk_stub.go for implementations.
func NewDiskManager() DiskManager {
	return newPlatformManager()
}
