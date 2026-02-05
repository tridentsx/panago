package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tridentsx/panago/pkg/firmware"
)

func main() {
	// Global flags
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "decode", "extract":
		if len(args) < 3 {
			fmt.Println("Usage: firmware decode <input.FRM> <output_dir>")
			os.Exit(1)
		}
		inputPath := args[1]
		outputDir := args[2]

		decoder := firmware.NewDecoder(*verbose)
		if err := decoder.DecodeFile(inputPath, outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Extraction complete.")

	case "encode", "pack":
		if len(args) < 4 {
			fmt.Println("Usage: firmware encode <input_dir> <output.FRM> <template.FRM>")
			os.Exit(1)
		}
		inputDir := args[1]
		outputPath := args[2]
		templatePath := args[3]

		encoder := firmware.NewEncoder(*verbose)
		if err := encoder.EncodeFile(inputDir, outputPath, templatePath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Encoding complete.")

	case "info":
		if len(args) < 2 {
			fmt.Println("Usage: firmware info <input.FRM>")
			os.Exit(1)
		}
		inputPath := args[1]

		decoder := firmware.NewDecoder(false)
		info, err := decoder.GetInfo(inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Firmware: %s\n", inputPath)
		fmt.Printf("Size: %d bytes\n", info.TotalSize)
		fmt.Printf("Partitions: %d\n", len(info.Partitions))
		fmt.Println()
		fmt.Printf("%-8s %-8s %12s %12s\n", "Name", "Version", "Offset", "Size")
		fmt.Println("-------- -------- ------------ ------------")
		for _, p := range info.Partitions {
			fmt.Printf("%-8s %-8s %12d %12d\n", p.Name, p.Version, p.Offset, p.Size)
		}

	case "test-crypto":
		// Test crypto round-trip
		testData := []byte("Hello, World! This is a test of the Feistel cipher.")
		// Pad to 8-byte boundary
		for len(testData)%8 != 0 {
			testData = append(testData, 0)
		}

		original := make([]byte, len(testData))
		copy(original, testData)

		f := firmware.NewFeistelCipher()
		f.Encrypt(testData)
		f.Decrypt(testData)

		match := true
		for i := range original {
			if original[i] != testData[i] {
				match = false
				break
			}
		}

		if match {
			fmt.Println("Feistel cipher round-trip: PASS")
		} else {
			fmt.Println("Feistel cipher round-trip: FAIL")
			os.Exit(1)
		}

		// Test AES
		aesTest := []byte("0123456789ABCDEF0123456789ABCDEF") // 32 bytes
		encrypted, _ := firmware.AESEncryptCBC(aesTest)
		decrypted, _ := firmware.AESDecryptCBC(encrypted)

		aesMatch := true
		for i := range aesTest {
			if aesTest[i] != decrypted[i] {
				aesMatch = false
				break
			}
		}

		if aesMatch {
			fmt.Println("AES-128-CBC round-trip: PASS")
		} else {
			fmt.Println("AES-128-CBC round-trip: FAIL")
			os.Exit(1)
		}

		// Test LZSS
		lzssTest := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA") // Highly compressible
		compressed := firmware.CompressLZSS(lzssTest)
		decompressed := firmware.DecompressLZSS(compressed)

		lzssMatch := len(lzssTest) == len(decompressed)
		if lzssMatch {
			for i := range lzssTest {
				if lzssTest[i] != decompressed[i] {
					lzssMatch = false
					break
				}
			}
		}

		if lzssMatch {
			fmt.Printf("LZSS round-trip: PASS (40 -> %d -> 40 bytes)\n", len(compressed))
		} else {
			fmt.Println("LZSS round-trip: FAIL")
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Panasonic Firmware Tool")
	fmt.Println()
	fmt.Println("Usage: firmware [flags] <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  decode <input.FRM> <output_dir>              Extract firmware partitions")
	fmt.Println("  encode <input_dir> <output.FRM> <template>   Rebuild firmware from partitions")
	fmt.Println("  info <input.FRM>                             Show firmware information")
	fmt.Println("  test-crypto                                  Test crypto implementations")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -v    Verbose output")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  firmware decode PANAEUSB.FRM ./extracted/")
	fmt.Println("  firmware encode ./modified/ PANAEUSB_new.FRM PANAEUSB.FRM")
	fmt.Println("  firmware info PANAEUSB.FRM")
}
