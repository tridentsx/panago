package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tridentsx/panago/pkg/cramfs"
)

func main() {
	endianFlag := flag.String("endian", "little", "Endianness (little or big)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("Usage: cramfs-extract <list|extract|extract-all|compress> <cramfs image or root folder> [output dir]")
		os.Exit(1)
	}

	command := args[0]
	imagePath := args[1]

	cfg := cramfs.DefaultConfig()
	if strings.ToLower(*endianFlag) == "big" {
		cfg.Endianness = binary.BigEndian
	}

	if command == "list" {
		err := cramfs.ListFiles(imagePath, cfg)
		if err != nil {
			fmt.Printf("Error listing files: %v\n", err)
			os.Exit(1)
		}
	} else if command == "extract" {
		if len(args) < 3 {
			fmt.Println("Missing output directory for extraction.")
			os.Exit(1)
		}
		outputPath := args[2]
		err := cramfs.ExtractFile(imagePath, outputPath, cfg)
		if err != nil {
			fmt.Printf("Error extracting files: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Extraction complete.")
	} else if command == "extract-all" {
		if len(args) < 3 {
			fmt.Println("Missing output directory for extraction.")
			os.Exit(1)
		}
		outputDir := args[2]
		err := cramfs.ExtractAll(imagePath, outputDir, cfg)
		if err != nil {
			fmt.Printf("Error extracting all files: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Full extraction complete.")
	} else if command == "compress" {
		if len(args) < 3 {
			fmt.Println("Missing output file for Cramfs image.")
			os.Exit(1)
		}
		outputFile := args[2]
		err := cramfs.CompressToCramfs(imagePath, outputFile, cfg)
		if err != nil {
			fmt.Printf("Error compressing directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Compression complete.")
	} else {
		fmt.Println("Invalid command. Use 'list', 'extract', 'extract-all', or 'compress'.")
		os.Exit(1)
	}
}
