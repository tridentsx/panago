package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tridentsx/panago/pkg/firmware"
)

var firmwareVerbose bool

var firmwareCmd = &cobra.Command{
	Use:   "firmware",
	Short: "Panasonic firmware tools",
	Long:  "Tools for decoding, encoding, and analyzing Panasonic DP-UB9000 firmware files",
}

var firmwareDecodeCmd = &cobra.Command{
	Use:   "decode <input.FRM> <output_dir>",
	Short: "Extract firmware partitions",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		decoder := firmware.NewDecoder(firmwareVerbose)
		return decoder.DecodeFile(args[0], args[1])
	},
}

var firmwareEncodeCmd = &cobra.Command{
	Use:   "encode <input_dir> <output.FRM> <template.FRM>",
	Short: "Rebuild firmware from partitions",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		encoder := firmware.NewEncoder(firmwareVerbose)
		return encoder.EncodeFile(args[0], args[1], args[2])
	},
}

var firmwareInfoCmd = &cobra.Command{
	Use:   "info <input.FRM>",
	Short: "Show firmware information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		decoder := firmware.NewDecoder(false)
		info, err := decoder.GetInfo(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Firmware: %s\n", args[0])
		fmt.Printf("Size: %d bytes\n", info.TotalSize)
		fmt.Printf("Partitions: %d\n", len(info.Partitions))
		fmt.Println()
		fmt.Printf("%-8s %-8s %12s %12s\n", "Name", "Version", "Offset", "Size")
		fmt.Println("-------- -------- ------------ ------------")
		for _, p := range info.Partitions {
			fmt.Printf("%-8s %-8s %12d %12d\n", p.Name, p.Version, p.Offset, p.Size)
		}
		return nil
	},
}

var firmwareSplitMainCmd = &cobra.Command{
	Use:   "split-main <MAIN.bin> <output_dir>",
	Short: "Split MAIN.bin into component images (fma4, fma5, fma6, fma7)",
	Long: `Splits the MAIN.bin partition into its component sub-images.

MAIN.bin contains concatenated images:
  - fma4: Kernel (raw binary)
  - fma5: Root filesystem (cramfs)
  - fma6: Data partition (romfs)
  - fma7: Application filesystem (cramfs)

After splitting, you can extract/modify fma5 with:
  panago-cli cramfs extract fma5.bin ./rootfs/
  # ... modify files ...
  panago-cli cramfs create ./rootfs/ fma5_modified.bin`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Splitting MAIN.bin...")
		_, err := firmware.SplitMainBin(args[0], args[1])
		return err
	},
}

var firmwareCombineMainCmd = &cobra.Command{
	Use:   "combine-main <input_dir> <MAIN.bin>",
	Short: "Combine fma4-fma7 back into MAIN.bin",
	Long: `Combines the component images back into a single MAIN.bin.

The input directory must contain:
  - fma4.bin
  - fma5.bin
  - fma6.bin
  - fma7.bin

These files are concatenated in order to create MAIN.bin.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Combining MAIN.bin...")
		return firmware.CombineMainBin(args[0], args[1])
	},
}

var firmwareTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test crypto implementations",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Test Feistel
		testData := []byte("Hello, World! This is a test of the Feistel cipher.")
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
		aesTest := []byte("0123456789ABCDEF0123456789ABCDEF")
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
		lzssTest := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
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

		return nil
	},
}

func init() {
	firmwareCmd.PersistentFlags().BoolVarP(&firmwareVerbose, "verbose", "v", false, "Verbose output")

	firmwareCmd.AddCommand(firmwareDecodeCmd)
	firmwareCmd.AddCommand(firmwareEncodeCmd)
	firmwareCmd.AddCommand(firmwareInfoCmd)
	firmwareCmd.AddCommand(firmwareSplitMainCmd)
	firmwareCmd.AddCommand(firmwareCombineMainCmd)
	firmwareCmd.AddCommand(firmwareTestCmd)

	rootCmd.AddCommand(firmwareCmd)
}
