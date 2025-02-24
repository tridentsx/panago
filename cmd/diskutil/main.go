package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
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
		err := formatDiskWithProgress(manager, disk, updateProgress)
		if err != nil {
			updateProgress("[red]Formatting Failed ❌[white]")
			time.Sleep(2 * time.Second)
			app.Stop()
			return
		}

		updateProgress("Extracting Tar Archive... 📦")
		tarFile := "backup.tar"
		err = extractTarWithProgress(manager, disk, tarFile, updateProgress)
		if err != nil {
			updateProgress("[red]Extraction Failed ❌[white]")
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

// Extract Tar Archive with Real-Time UI Updates
func extractTarWithProgress(manager DiskManager, disk Disk, tarFile string, update func(string)) error {
	cmd := exec.Command("tar", "-xvf", tarFile, "-C", disk.DevicePath)
	stdoutPipe, _ := cmd.StdoutPipe()
	cmd.Start()

	scanner := bufio.NewScanner(stdoutPipe)
	count := 0
	for scanner.Scan() {
		count++
		update(fmt.Sprintf("Extracting Data... %d files", count))
	}

	cmd.Wait()
	update("Extraction Complete ✅")
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
