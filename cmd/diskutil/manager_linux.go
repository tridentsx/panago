//go:build linux
// +build linux

package main

func init() {
	diskManager = LinuxDiskManager{}
}
