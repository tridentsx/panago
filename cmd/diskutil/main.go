package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/klauspost/compress/gzip"
	"github.com/rivo/tview"
)

// runtime selection is handled by init functions in platform-specific files

func main() {
	manager := getDiskManager()

	// Step 1: List USB Disks
	disks, err := manager.ListUSBDisks()
	if err != nil || len(disks) == 0 {
		fmt.Println("Error: No USB disks found.")
		return
	}

	// Step 2: Create UI app
	app := tview.NewApplication()
	list := tview.NewList().ShowSecondaryText(false)
	list.SetTitle(" Select USB Drive ").SetBorder(true)

	// Add disks to the menu
	for i, disk := range disks {
		disk := disk // Capture variable for closure
		list.AddItem(fmt.Sprintf("[%d] %s (%s, %s)", i, disk.Name, disk.Model, disk.Size), "", 0, func() {
			app.Stop()
			startProgressUI(manager, disk)
		})
	}

	// Step 3: Run UI
	if err := app.SetRoot(list, true).Run(); err != nil {
		fmt.Println("Failed to start UI:", err)
	}
}

// UI for Progress
func startProgressUI(manager DiskManager, disk Disk) {
	app := tview.NewApplication()
	progressText := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	progressText.SetBorder(true).SetTitle(" Progress ")

	// Function to update progress
	updateProgress := func(text string) {
		app.QueueUpdateDraw(func() {
			progressText.SetText(text)
		})
	}

	// Step 1: Confirmation Prompt
	var confirm string
	prompt := &survey.Select{
		Message: fmt.Sprintf("WARNING: Formatting %s will erase all data. Are you sure?", disk.DevicePath),
		Options: []string{"YES", "NO"},
	}
	survey.AskOne(prompt, &confirm)
	if confirm != "YES" {
		fmt.Println("Operation canceled.")
		return
	}

	// Step 2: Start UI Progress
	grid := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(progressText, 0, 1, false)

	app.SetRoot(grid, true)

	// Run UI in separate goroutine
	go func() {
		updateProgress("Formatting Disk... ⏳")
		err := manager.FormatDisk(disk)
		if err != nil {
			updateProgress("[red]Formatting Failed ❌[white]")
			time.Sleep(2 * time.Second)
			app.Stop()
			return
		}

		updateProgress("Writing Disk Image... 📦")
		imageFile := "drive.img.gz" // Use your actual image file name
		err = extractImageWithProgress(manager, disk, imageFile, updateProgress)
		if err != nil {
			updateProgress("[red]Image Writing Failed ❌[white]: " + err.Error())
			time.Sleep(2 * time.Second)
			app.Stop()
			return
		}

		updateProgress("[green]Operation Completed Successfully ✅[white]")
		time.Sleep(3 * time.Second)
		app.Stop()
	}()

	// Run UI
	if err := app.Run(); err != nil {
		fmt.Println("Error running UI:", err)
	}
}

// Format Disk with Real-Time UI Updates
func formatDiskWithProgress(manager DiskManager, disk Disk, update func(string)) error {
	cmd := exec.Command("mkfs.ext4", "-v", disk.DevicePath) // Use proper OS command

	stdoutPipe, _ := cmd.StdoutPipe()
	cmd.Start()

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Print to CLI for debugging

		// Detect progress percentage
		if strings.Contains(line, "%") {
			parts := strings.Fields(line)
			progress := parseInt(parts[len(parts)-1])
			update(fmt.Sprintf("Formatting Disk... %d%%", progress))
		}
	}

	cmd.Wait()
	update("Formatting Complete ✅")
	return nil
}

// Extract Image with Real-Time UI Updates
func extractImageWithProgress(manager DiskManager, disk Disk, imageFile string, update func(string)) error {
	// Check if we're on Windows using runtime.GOOS
	if runtime.GOOS == "windows" {
		return extractImageWindowsWithProgress(disk, imageFile, update)
	}

	// For Linux and macOS, use the existing implementation
	// First, decompress the gzip file if needed
	var sourceFile string
	if strings.HasSuffix(imageFile, ".gz") {
		update("Decompressing image file... 📦")
		sourceFile = strings.TrimSuffix(imageFile, ".gz")
		cmd := exec.Command("gunzip", "-c", imageFile)
		outFile, err := os.Create(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to create decompressed file: %w", err)
		}
		defer outFile.Close()

		cmd.Stdout = outFile
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to decompress image: %w", err)
		}
		update("Decompression complete ✅")
	} else {
		sourceFile = imageFile
	}

	// Now write the raw image to the disk
	update("Writing disk image to USB drive... 📀")

	// Use dd to write the image
	cmd := exec.Command("dd", "if="+sourceFile, "of="+disk.DevicePath, "bs=4M", "status=progress")
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start dd command: %w", err)
	}

	// Combine stdout and stderr for progress monitoring
	scanner := bufio.NewScanner(io.MultiReader(stdoutPipe, stderrPipe))
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Print to CLI for debugging

		// Try to extract progress information
		if strings.Contains(line, "bytes") {
			update(fmt.Sprintf("Writing image... %s", line))
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("dd command failed: %w", err)
	}

	update("Image writing complete ✅")

	// Clean up the decompressed file if we created it
	if imageFile != sourceFile {
		os.Remove(sourceFile)
	}

	return nil
}

// Add this function to handle Windows-specific image writing
func extractImageWindowsWithProgress(disk Disk, imageFile string, update func(string)) error {
	// First, decompress the gzip file if needed
	var sourceFile string
	if strings.HasSuffix(imageFile, ".gz") {
		update("Decompressing image file... 📦")
		sourceFile = strings.TrimSuffix(imageFile, ".gz")

		// Use Go's built-in gzip package instead of PowerShell
		gzipFile, err := os.Open(imageFile)
		if err != nil {
			return fmt.Errorf("failed to open gzip file: %w", err)
		}
		defer gzipFile.Close()

		gzipReader, err := gzip.NewReader(gzipFile)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()

		outFile, err := os.Create(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()

		// Copy with progress reporting
		totalSize := int64(0)               // We don't know the uncompressed size in advance
		buffer := make([]byte, 4*1024*1024) // 4MB buffer
		for {
			n, err := gzipReader.Read(buffer)
			if err != nil && err != io.EOF {
				return fmt.Errorf("error reading from gzip: %w", err)
			}
			if n == 0 {
				break
			}

			if _, err := outFile.Write(buffer[:n]); err != nil {
				return fmt.Errorf("error writing to output file: %w", err)
			}

			totalSize += int64(n)
			update(fmt.Sprintf("Decompressing... %d MB written", totalSize/(1024*1024)))
		}

		update("Decompression complete ✅")
	} else {
		sourceFile = imageFile
	}

	// Now write the raw image to the disk using PowerShell and Win32 APIs
	update("Writing disk image to USB drive... 📀")

	// Create a PowerShell script to write the image
	scriptContent := `
	param($imagePath, $devicePath)
	
	$bytes = [System.IO.File]::ReadAllBytes($imagePath)
	$device = New-Object System.IO.FileStream($devicePath, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Write)
	
	$totalSize = $bytes.Length
	$chunkSize = 1MB
	$written = 0
	
	for ($i = 0; $i -lt $totalSize; $i += $chunkSize) {
		$remaining = $totalSize - $i
		$toWrite = [Math]::Min($chunkSize, $remaining)
		$device.Write($bytes, $i, $toWrite)
		$written += $toWrite
		$percent = [Math]::Round(($written / $totalSize) * 100)
		Write-Host "Progress: $percent% ($written / $totalSize bytes)"
	}
	
	$device.Close()
	Write-Host "Write complete"
	`

	// Save the script to a temporary file
	scriptFile := "write_image.ps1"
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("failed to create PowerShell script: %w", err)
	}
	defer os.Remove(scriptFile)

	// Run the PowerShell script
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptFile,
		"-imagePath", sourceFile, "-devicePath", disk.DevicePath)

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start PowerShell script: %w", err)
	}

	// Monitor progress
	scanner := bufio.NewScanner(io.MultiReader(stdoutPipe, stderrPipe))
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Print to CLI for debugging

		if strings.Contains(line, "Progress:") {
			update(fmt.Sprintf("Writing image... %s", line))
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("PowerShell script failed: %w", err)
	}

	update("Image writing complete ✅")

	// Clean up the decompressed file if we created it
	if imageFile != sourceFile {
		os.Remove(sourceFile)
	}

	return nil
}

// Helper Function: Convert String to Int
func parseInt(s string) int {
	val, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
	if err != nil {
		return 0
	}
	return val
}
