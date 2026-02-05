package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/pkg/cramfs"
	"github.com/tridentsx/panago/pkg/firmware"
	"github.com/tridentsx/panago/pkg/romfs"
)

var extractCmd = &cobra.Command{
	Use:   "extract <firmware.FRM> <output_dir>",
	Short: "Extract firmware and all filesystems in one step",
	Long: `Extracts a Panasonic firmware file and all its components:

1. Decrypts and extracts firmware partitions
2. Splits MAIN.bin into fma4, fma5, fma6, fma7
3. Extracts fma5 (cramfs) to fma5/ directory
4. Extracts fma6 (romfs) to fma6/ directory
5. Extracts fma7 (cramfs) to fma7/ directory

Output structure:
  output_dir/
  ├── PROG_*.bin, MINI_*.bin, etc. (other partitions)
  ├── fma4.bin                     (kernel, kept as binary)
  ├── fma5/                        (root filesystem)
  │   ├── sbin/init
  │   └── ...
  ├── fma6/                        (data partition)
  │   └── ...
  └── fma7/                        (app filesystem)
      └── ...

After extraction, modify files in fma5/, fma6/, or fma7/ directories,
then use 'panago-cli build' to create a new firmware.`,
	Args: cobra.ExactArgs(2),
	RunE: runExtract,
}

var buildCmd = &cobra.Command{
	Use:   "build <workspace_dir> <output.FRM> <template.FRM>",
	Short: "Build firmware from extracted workspace",
	Long: `Builds a new firmware file from a previously extracted workspace:

1. Rebuilds fma5.bin from fma5/ directory (cramfs)
2. Rebuilds fma6.bin from fma6/ directory (romfs)
3. Rebuilds fma7.bin from fma7/ directory (cramfs)
4. Combines fma4-7 into MAIN.bin
5. Encrypts and packages final firmware

The template firmware is used to preserve the original structure
and encryption. Only modified partitions are replaced.`,
	Args: cobra.ExactArgs(3),
	RunE: runBuild,
}

func init() {
	rootCmd.AddCommand(extractCmd)
	rootCmd.AddCommand(buildCmd)
}

func runExtract(cmd *cobra.Command, args []string) error {
	firmwarePath := args[0]
	outputDir := args[1]

	fmt.Printf("Extracting %s to %s/\n\n", firmwarePath, outputDir)

	// Step 1: Decode firmware
	fmt.Println("Step 1: Decoding firmware...")
	decoder := firmware.NewDecoder(false)
	if err := decoder.DecodeFile(firmwarePath, outputDir); err != nil {
		return fmt.Errorf("failed to decode firmware: %w", err)
	}

	// Step 2: Split MAIN.bin
	mainPath := filepath.Join(outputDir, "MAIN.bin")
	if _, err := os.Stat(mainPath); err == nil {
		fmt.Println("\nStep 2: Splitting MAIN.bin...")
		parts, err := firmware.SplitMainBin(mainPath, outputDir)
		if err != nil {
			return fmt.Errorf("failed to split MAIN.bin: %w", err)
		}

		// Remove MAIN.bin after splitting (we'll rebuild it)
		os.Remove(mainPath)

		// Step 3: Extract filesystems
		fmt.Println("\nStep 3: Extracting filesystems...")
		cramfsCfg := cramfs.DefaultConfig()
		romfsCfg := romfs.DefaultConfig()

		for _, p := range parts {
			partPath := filepath.Join(outputDir, p.Name+".bin")
			extractDir := filepath.Join(outputDir, p.Name)

			switch p.Type {
			case "cramfs":
				fmt.Printf("  Extracting %s (cramfs)...\n", p.Name)
				if err := cramfs.ExtractAll(partPath, extractDir, cramfsCfg); err != nil {
					return fmt.Errorf("failed to extract %s: %w", p.Name, err)
				}
				// Remove the .bin file, keep the directory
				os.Remove(partPath)

			case "romfs":
				fmt.Printf("  Extracting %s (romfs)...\n", p.Name)
				if err := romfs.ExtractAll(partPath, extractDir, romfsCfg); err != nil {
					return fmt.Errorf("failed to extract %s: %w", p.Name, err)
				}
				// Remove the .bin file, keep the directory
				os.Remove(partPath)

			case "raw":
				fmt.Printf("  Keeping %s as binary\n", p.Name)
				// Keep fma4.bin as-is (kernel)
			}
		}
	}

	fmt.Printf("\nExtraction complete!\n")
	fmt.Printf("\nWorkspace structure:\n")
	fmt.Printf("  %s/\n", outputDir)
	fmt.Printf("  ├── PROG_*.bin, MINI_*.bin, etc.\n")
	fmt.Printf("  ├── fma4.bin  (kernel)\n")
	fmt.Printf("  ├── fma5/     (root filesystem - editable)\n")
	fmt.Printf("  ├── fma6/     (data partition - editable)\n")
	fmt.Printf("  └── fma7/     (app filesystem - editable)\n")
	fmt.Printf("\nModify files in fma5/, fma6/, or fma7/, then run:\n")
	fmt.Printf("  panago-cli build %s output.FRM %s\n", outputDir, firmwarePath)

	return nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	workspaceDir := args[0]
	outputPath := args[1]
	templatePath := args[2]

	fmt.Printf("Building firmware from %s/\n\n", workspaceDir)

	cramfsCfg := cramfs.DefaultConfig()
	romfsCfg := romfs.DefaultConfig()

	// Step 1: Rebuild cramfs/romfs images
	fmt.Println("Step 1: Rebuilding filesystem images...")

	// Check which filesystems exist as directories
	fma5Dir := filepath.Join(workspaceDir, "fma5")
	fma6Dir := filepath.Join(workspaceDir, "fma6")
	fma7Dir := filepath.Join(workspaceDir, "fma7")

	fma5Bin := filepath.Join(workspaceDir, "fma5.bin")
	fma6Bin := filepath.Join(workspaceDir, "fma6.bin")
	fma7Bin := filepath.Join(workspaceDir, "fma7.bin")

	// Rebuild fma5 if directory exists
	if info, err := os.Stat(fma5Dir); err == nil && info.IsDir() {
		fmt.Printf("  Rebuilding fma5.bin (cramfs)...\n")
		if err := cramfs.CompressToCramfs(fma5Dir, fma5Bin, cramfsCfg); err != nil {
			return fmt.Errorf("failed to build fma5: %w", err)
		}
	}

	// Rebuild fma6 if directory exists
	if info, err := os.Stat(fma6Dir); err == nil && info.IsDir() {
		fmt.Printf("  Rebuilding fma6.bin (romfs)...\n")
		builder := romfs.NewBuilder("rom", romfsCfg)
		if err := builder.BuildToFile(fma6Dir, fma6Bin); err != nil {
			return fmt.Errorf("failed to build fma6: %w", err)
		}
	}

	// Rebuild fma7 if directory exists
	if info, err := os.Stat(fma7Dir); err == nil && info.IsDir() {
		fmt.Printf("  Rebuilding fma7.bin (cramfs)...\n")
		if err := cramfs.CompressToCramfs(fma7Dir, fma7Bin, cramfsCfg); err != nil {
			return fmt.Errorf("failed to build fma7: %w", err)
		}
	}

	// Step 2: Combine into MAIN.bin
	fmt.Println("\nStep 2: Combining MAIN.bin...")
	mainPath := filepath.Join(workspaceDir, "MAIN.bin")
	if err := firmware.CombineMainBin(workspaceDir, mainPath); err != nil {
		return fmt.Errorf("failed to combine MAIN.bin: %w", err)
	}

	// Step 3: Encode firmware
	fmt.Println("\nStep 3: Encoding firmware...")
	encoder := firmware.NewEncoder(false)
	if err := encoder.EncodeFile(workspaceDir, outputPath, templatePath); err != nil {
		return fmt.Errorf("failed to encode firmware: %w", err)
	}

	fmt.Printf("\nBuild complete!\n")
	fmt.Printf("Output: %s\n", outputPath)

	return nil
}
