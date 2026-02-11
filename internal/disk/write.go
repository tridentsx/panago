package disk

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/klauspost/compress/gzip"
)

// ExtractImageWithProgress writes a (possibly gzipped) disk image to a device.
// Used on Linux and macOS.
func ExtractImageWithProgress(disk Disk, imageFile string, update func(string)) error {
	if runtime.GOOS == "windows" {
		return ExtractImageWindowsWithProgress(disk, imageFile, update)
	}

	var sourceFile string
	if strings.HasSuffix(imageFile, ".gz") {
		update("Decompressing image file...")
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
		update("Decompression complete")
	} else {
		sourceFile = imageFile
	}

	update("Writing disk image to USB drive...")

	cmd := exec.Command("dd", "if="+sourceFile, "of="+disk.DevicePath, "bs=4M", "status=progress")
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start dd command: %w", err)
	}

	scanner := bufio.NewScanner(io.MultiReader(stdoutPipe, stderrPipe))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "bytes") {
			update(fmt.Sprintf("Writing image... %s", line))
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("dd command failed: %w", err)
	}

	update("Image writing complete")

	if imageFile != sourceFile {
		os.Remove(sourceFile)
	}

	return nil
}

// ExtractImageWindowsWithProgress writes a (possibly gzipped) disk image to a device on Windows.
func ExtractImageWindowsWithProgress(disk Disk, imageFile string, update func(string)) error {
	var sourceFile string
	if strings.HasSuffix(imageFile, ".gz") {
		update("Decompressing image file...")
		sourceFile = strings.TrimSuffix(imageFile, ".gz")

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

		totalSize := int64(0)
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

		update("Decompression complete")
	} else {
		sourceFile = imageFile
	}

	update("Writing disk image to USB drive...")

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

	scriptFile := "write_image.ps1"
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("failed to create PowerShell script: %w", err)
	}
	defer os.Remove(scriptFile)

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptFile,
		"-imagePath", sourceFile, "-devicePath", disk.DevicePath)

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start PowerShell script: %w", err)
	}

	scanner := bufio.NewScanner(io.MultiReader(stdoutPipe, stderrPipe))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Progress:") {
			update(fmt.Sprintf("Writing image... %s", line))
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("PowerShell script failed: %w", err)
	}

	update("Image writing complete")

	if imageFile != sourceFile {
		os.Remove(sourceFile)
	}

	return nil
}
