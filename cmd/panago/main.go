package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/rivo/tview"
	"github.com/tridentsx/panago/internal/shell"
)

// Build-time variable via -ldflags (optional)
var version = "dev"

// findPatchFolders scans the current directory for folders named "patch_XXX" and
// returns a map: folderName => displayVersion (e.g. "patch_169" => "1.69").
func findPatchFolders() map[string]string {
	result := make(map[string]string)

	files, err := ioutil.ReadDir(".")
	if err != nil {
		return result
	}

	for _, f := range files {
		if f.IsDir() && strings.HasPrefix(f.Name(), "patch_") {
			// Example: "patch_169" => "1.69"
			numStr := strings.TrimPrefix(f.Name(), "patch_") // "169"
			if len(numStr) >= 2 {
				displayVersion := numStr[:1] + "." + numStr[1:] // e.g. "1.69"
				result[f.Name()] = displayVersion
			} else {
				// If there's some odd naming, just store as-is
				result[f.Name()] = numStr
			}
		}
	}
	return result
}

func main() {
	app := tview.NewApplication()

	// Create and configure the IP form
	var ipForm *tview.Form
	ipForm = tview.NewForm().
		AddInputField("IP Address", "", 20, nil, nil).
		AddButton("Connect", func() {
			ipAddr := ipForm.GetFormItemByLabel("IP Address").(*tview.InputField).GetText()
			if ipAddr == "" {
				showModal(app, "Error", "IP Address cannot be empty.", func() {
					app.SetRoot(ipForm, true)
				})
				return
			}

			// Try to run the exploit logic:
			err := runExploitLogic(ipAddr)
			if err != nil {
				showModal(app, "Error", err.Error(), func() {
					// On modal dismiss, return to the IP form
					app.SetRoot(ipForm, true)
				})
				return
			}

			// If success, show main menu
			showMainMenu(app, ipAddr)
		}).
		AddButton("Quit", func() {
			app.Stop()
		})

	ipForm.SetTitle(fmt.Sprintf(" panago v%s ", version)).
		SetBorder(true).
		SetBorderPadding(1, 1, 2, 2)

	app.SetRoot(ipForm, true).SetFocus(ipForm)

	// Start the application
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// runExploitLogic checks ports and sends payloads
func runExploitLogic(ipAddress string) error {
	// Create a new shell session
	sh, err := shell.New(ipAddress)
	if err != nil {
		return fmt.Errorf("failed to start remote shell: %w", err)
	}
	defer sh.Close()

	// Test the connection with a simple command
	if err := sh.ExecuteCommand("echo ok"); err != nil {
		return fmt.Errorf("failed to verify shell connection: %w", err)
	}

	// Read response to verify connection
	output, err := sh.GetOutput()
	if err != nil {
		return fmt.Errorf("failed to verify shell response: %w", err)
	}

	// Read a small amount to verify connection
	buf := make([]byte, 1024)
	n, err := output.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read shell response: %w", err)
	}

	if !strings.Contains(string(buf[:n]), "ok") {
		return fmt.Errorf("invalid shell response")
	}

	return nil
}

// Global menu variable to ensure proper scope in closures
var mainMenu *tview.List

// showMainMenu creates a TUI menu to choose between Backup, Patch, or Quit.
func showMainMenu(app *tview.Application, ipAddr string) {
	mainMenu = tview.NewList()
	mainMenu.AddItem("Backup Player", "", 'b', func() {
		err := runExploitLogic(ipAddr)
		if err != nil {
			showModal(app, "Error", err.Error(), func() {
				app.SetRoot(mainMenu, true)
			})
			return
		}
		showModal(app, "Success", "Operation completed successfully!", func() {
			app.SetRoot(mainMenu, true)
		})
	})
	mainMenu.AddItem("Patch Player", "", 'p', func() {
		showPatchMenu(app, mainMenu)
	})
	mainMenu.AddItem("Quit", "", 'q', func() {
		app.Stop()
	})

	mainMenu.SetTitle(" Choose an action ")
	mainMenu.SetBorder(true)
	mainMenu.SetBorderPadding(1, 1, 2, 2)

	app.SetRoot(mainMenu, true).SetFocus(mainMenu)
}

// showPatchMenu lists all patch folders ("patch_168", etc.) and displays them
// as items (1.68, 1.69, etc.). Selecting one can trigger your patch logic.
func showPatchMenu(app *tview.Application, prevPage tview.Primitive) {
	patchMap := findPatchFolders()
	if len(patchMap) == 0 {
		showModal(app, "No patches found", "No patch_* folders in current directory.", func() {
			app.SetRoot(prevPage, true)
		})
		return
	}

	// Initialize patchList
	var patchList *tview.List = tview.NewList()
	patchList.SetTitle(" Available patches ")
	patchList.SetBorder(true)
	patchList.SetBorderPadding(1, 1, 2, 2)

	for folderName, displayVersion := range patchMap {
		f := folderName
		dv := displayVersion
		patchList.AddItem(dv, "", 0, func() {
			// PATCH logic placeholder
			msg := fmt.Sprintf("Patch %s selected. (Folder: %s)", dv, f)
			showModal(app, "Patch selected", msg, func() {
				// Return to main menu
				app.SetRoot(prevPage, true)
			})
		})
	}

	patchList.AddItem("Back", "", 'b', func() {
		app.SetRoot(prevPage, true)
	})

	app.SetRoot(patchList, true).SetFocus(patchList)
}

// showModal is a helper to display a message box with an "OK" button.
func showModal(app *tview.Application, title, message string, onDismiss func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			onDismiss()
		})

	modal.SetTitle(" " + title + " ").
		SetBorder(true)

	app.SetRoot(modal, true).SetFocus(modal)
}
