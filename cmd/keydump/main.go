package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tridentsx/panago/internal/keydump"
)

func main() {
	ip := flag.String("ip", "", "Target player IP address")
	libPath := flag.String("lib", "", "Path to libfmupre.so (will download from device if not specified)")
	patchOnly := flag.Bool("patch-only", false, "Only generate patched library, don't extract keys")
	output := flag.String("output", "libfmupre_patched.so", "Output path for patched library")
	flag.Parse()

	if *patchOnly {
		if *libPath == "" {
			fmt.Println("Error: -lib required with -patch-only")
			os.Exit(1)
		}
		fmt.Printf("Patching %s -> %s\n", *libPath, *output)
		if err := keydump.PatchLibrary(*libPath, *output); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Done! Patched library will dump keys to /tmp/fpc_keys.bin")
		return
	}

	if *ip == "" {
		fmt.Println("Panago FPC Key Extractor")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  keydump -ip <player_ip>              Extract keys from device")
		fmt.Println("  keydump -patch-only -lib <path>      Generate patched library only")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	fmt.Printf("[*] Connecting to %s...\n", *ip)
	kd, err := keydump.New(*ip)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer kd.Close()

	// If no local lib provided, we need to download from device
	localLib := *libPath
	if localLib == "" {
		localLib = "libfmupre.so"
		fmt.Println("[*] Downloading libfmupre.so from device...")
		// TODO: implement download
		fmt.Println("    Note: Download not implemented yet, please provide -lib")
		os.Exit(1)
	}

	fmt.Println("[*] Patching library...")
	if err := keydump.PatchLibrary(localLib, *output); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[*] Extracting keys...")
	k1, k2, err := kd.ExtractKeys(*output)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== FPC KEYS ===")
	fmt.Printf("K1: %x\n", k1)
	fmt.Printf("K2: %x\n", k2)
	fmt.Println()

	// Save to file
	keyFile := "fpc_keys.bin"
	f, err := os.Create(keyFile)
	if err == nil {
		f.Write(k1)
		f.Write(k2)
		f.Close()
		fmt.Printf("Keys saved to %s\n", keyFile)
	}
}
