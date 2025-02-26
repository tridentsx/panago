//go:build darwin
// +build darwin

package main

func init() {
	diskManager = MacOSDiskManager{}
}
