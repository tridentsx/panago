package main

// defaultManager holds the platform-specific disk manager instance
var defaultManager DiskManager

// getDiskManager returns the platform-specific disk manager instance
func getDiskManager() DiskManager {
	if defaultManager == nil {
		panic("No disk manager implementation available for this platform")
	}
	return defaultManager
}