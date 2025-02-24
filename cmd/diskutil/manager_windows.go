//go:build windows
// +build windows

package main

func init() {
	defaultManager = WindowsDiskManager{}
}