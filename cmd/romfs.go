package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/pkg/romfs"
)

var romfsVolumeName string

var romfsCmd = &cobra.Command{
	Use:   "romfs",
	Short: "Romfs filesystem tools",
	Long:  "Tools for extracting and creating romfs filesystem images",
}

var romfsListCmd = &cobra.Command{
	Use:   "list <image.bin>",
	Short: "List files in romfs image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := romfs.DefaultConfig()
		return romfs.ListFiles(args[0], cfg)
	},
}

var romfsExtractCmd = &cobra.Command{
	Use:   "extract <image.bin> <output_dir>",
	Short: "Extract all files from romfs image",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := romfs.DefaultConfig()
		if err := romfs.ExtractAll(args[0], args[1], cfg); err != nil {
			return err
		}
		fmt.Println("Extraction complete.")
		return nil
	},
}

var romfsCreateCmd = &cobra.Command{
	Use:   "create <input_dir> <output.bin>",
	Short: "Create romfs image from directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := romfs.DefaultConfig()
		builder := romfs.NewBuilder(romfsVolumeName, cfg)
		if err := builder.BuildToFile(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Created romfs image: %s\n", args[1])
		return nil
	},
}

var romfsInfoCmd = &cobra.Command{
	Use:   "info <image.bin>",
	Short: "Show romfs image information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := romfs.DefaultConfig()
		ext, err := romfs.NewExtractor(args[0], cfg)
		if err != nil {
			return err
		}

		super := ext.GetSuperblock()
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
			} else if f.IsFile {
				fileCount++
			}
		}

		fmt.Printf("Romfs image: %s\n", args[0])
		fmt.Printf("Volume name: %s\n", super.Name)
		fmt.Printf("Image size: %d bytes\n", super.Size)
		fmt.Printf("Files: %d\n", fileCount)
		fmt.Printf("Directories: %d\n", dirCount)
		fmt.Printf("Symlinks: %d\n", linkCount)
		fmt.Printf("Total content size: %d bytes\n", totalSize)

		return nil
	},
}

func init() {
	romfsCreateCmd.Flags().StringVarP(&romfsVolumeName, "name", "n", "rom", "Volume name for the romfs image")

	romfsCmd.AddCommand(romfsListCmd)
	romfsCmd.AddCommand(romfsExtractCmd)
	romfsCmd.AddCommand(romfsCreateCmd)
	romfsCmd.AddCommand(romfsInfoCmd)

	rootCmd.AddCommand(romfsCmd)
}
