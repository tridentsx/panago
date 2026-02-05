package cmd

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/pkg/cramfs"
)

var cramfsEndian string

var cramfsCmd = &cobra.Command{
	Use:   "cramfs",
	Short: "Cramfs filesystem tools",
	Long:  "Tools for extracting and creating cramfs filesystem images",
}

var cramfsListCmd = &cobra.Command{
	Use:   "list <image.bin>",
	Short: "List files in cramfs image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getCramfsConfig()
		return cramfs.ListFiles(args[0], cfg)
	},
}

var cramfsExtractCmd = &cobra.Command{
	Use:   "extract <image.bin> <output_dir>",
	Short: "Extract all files from cramfs image",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getCramfsConfig()
		if err := cramfs.ExtractAll(args[0], args[1], cfg); err != nil {
			return err
		}
		fmt.Println("Extraction complete.")
		return nil
	},
}

var cramfsCreateCmd = &cobra.Command{
	Use:   "create <input_dir> <output.bin>",
	Short: "Create cramfs image from directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getCramfsConfig()
		return cramfs.CompressToCramfs(args[0], args[1], cfg)
	},
}

var cramfsInfoCmd = &cobra.Command{
	Use:   "info <image.bin>",
	Short: "Show cramfs image information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := getCramfsConfig()
		ext, err := cramfs.NewExtractor(args[0], cfg)
		if err != nil {
			return err
		}

		files, err := ext.ListFiles()
		if err != nil {
			return err
		}

		var totalSize uint32
		var fileCount, dirCount, linkCount int
		for _, f := range files {
			totalSize += f.Size
			if f.IsDir {
				dirCount++
			} else if f.IsLink {
				linkCount++
			} else {
				fileCount++
			}
		}

		fmt.Printf("Cramfs image: %s\n", args[0])
		fmt.Printf("Files: %d\n", fileCount)
		fmt.Printf("Directories: %d\n", dirCount)
		fmt.Printf("Symlinks: %d\n", linkCount)
		fmt.Printf("Total uncompressed size: %d bytes\n", totalSize)

		return nil
	},
}

func getCramfsConfig() *cramfs.Config {
	cfg := cramfs.DefaultConfig()
	if strings.ToLower(cramfsEndian) == "big" {
		cfg.Endianness = binary.BigEndian
	}
	return cfg
}

func init() {
	cramfsCmd.PersistentFlags().StringVarP(&cramfsEndian, "endian", "e", "little", "Endianness (little or big)")

	cramfsCmd.AddCommand(cramfsListCmd)
	cramfsCmd.AddCommand(cramfsExtractCmd)
	cramfsCmd.AddCommand(cramfsCreateCmd)
	cramfsCmd.AddCommand(cramfsInfoCmd)

	rootCmd.AddCommand(cramfsCmd)
}
