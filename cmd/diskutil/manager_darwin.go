//go:build darwin
// +build darwin

package main

func init() {
	defaultManager = MacOSDiskManager{}
}