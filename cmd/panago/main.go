package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/tridentsx/panago/internal"
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

	// Declare ipForm before using it
	ipForm := tview.NewForm().
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
			err := runExploitLogic(app, ipAddr)
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

// runExploitLogic checks ports 60030 & 2222, and sends the two payloads.
func runExploitLogic(app *tview.Application, ipAddress string) error {
	// Check if anything is listening on port 2222 first
	if internal.IsPort2222Open(ipAddress) {
		return fmt.Errorf("player at %s is already pawned on port 2222", ipAddress)
	}

	// Check if the player is on port 60030
	if !internal.IsPlayerAvailable(ipAddress) {
		return fmt.Errorf("no player detected on port 60030 for IP %s", ipAddress)
	}

	// Send first payload
	if err := internal.SendFirstPayload(ipAddress); err != nil {
		return fmt.Errorf("failed to send first payload: %v", err)
	}

	// Wait a moment before the second payload
	time.Sleep(1 * time.Second)

	// Check if punch binary is ready on port 2222
	if !internal.IsPort2222Open(ipAddress) {
		return fmt.Errorf("punch binary not ready on port 2222 for IP %s", ipAddress)
	}

	// Send second payload
	if err := internal.SendSecondPayload(ipAddress); err != nil {
		return fmt.Errorf("failed to send second payload: %v", err)
	}

	return nil
}

// showMainMenu creates a TUI menu to choose between Backup, Patch, or Quit.
func showMainMenu(app *tview.Application, ipAddr string) {
	// Declare menu before using it
	menu := tview.NewList().
		AddItem("Backup Player", "", 'b', func() {
			showModal(app, "Backup", "Backup completed successfully!", func() {
				// Return to main menu
				app.SetRoot(menu, true)
			})
		}).
		AddItem("Patch Player", "", 'p', func() {
			showPatchMenu(app, menu)
		}).
		AddItem("Quit", "", 'q', func() {
			app.Stop()
		})

	menu.SetTitle(" Choose an action ").
		SetBorder(true).
		SetBorderPadding(1, 1, 2, 2)

	app.SetRoot(menu, true).SetFocus(menu)
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

	// Change patchList to a `List` (fix issue)
	patchList := tview.NewList().SetTitle(" Available patches ").SetBorder(true)

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
