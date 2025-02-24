//go:build !windows && !linux && !darwin
// +build !windows,!linux,!darwin

package main

func (w WindowsDiskManager) ListUSBDisks() ([]Disk, error) { return nil, nil }
func (w WindowsDiskManager) FormatDisk(disk Disk) error { return nil }
func (w WindowsDiskManager) MountAndExtract(disk Disk, tarFile string) error { return nil }

func (l LinuxDiskManager) ListUSBDisks() ([]Disk, error) { return nil, nil }
func (l LinuxDiskManager) FormatDisk(disk Disk) error { return nil }
func (l LinuxDiskManager) MountAndExtract(disk Disk, tarFile string) error { return nil }

func (m MacOSDiskManager) ListUSBDisks() ([]Disk, error) { return nil, nil }
func (m MacOSDiskManager) FormatDisk(disk Disk) error { return nil }
func (m MacOSDiskManager) MountAndExtract(disk Disk, tarFile string) error { return nil }