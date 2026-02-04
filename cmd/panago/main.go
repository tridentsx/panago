package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/tridentsx/panago/internal/backup"
	"github.com/tridentsx/panago/internal/discover"
	"github.com/tridentsx/panago/internal/exploit"
	"github.com/tridentsx/panago/internal/keydump"
)

// Build-time variable via -ldflags (optional)
var version = "dev"

var (
	app    *tview.Application
	status *PlayerStatus
	flex   *tview.Flex
)

type PlayerStatus struct {
	ip        string
	detected  bool
	version   string
	exploited bool
	lastSeen  time.Time
}

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
	app = tview.NewApplication()
	status = &PlayerStatus{}

	// Create main layout
	flex = tview.NewFlex().SetDirection(tview.FlexRow)

	// Status panel at top
	statusPanel := createStatusPanel()
	flex.AddItem(statusPanel, 6, 0, false)

	// Main content area
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(content, 0, 1, true)

	// IP Input form
	ipForm := createIPForm(content)
	content.AddItem(ipForm, 0, 1, true)

	app.SetRoot(flex, true).EnableMouse(true)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

func createStatusPanel() *tview.TextView {
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	// Update status periodically
	go func() {
		for {
			app.QueueUpdateDraw(func() {
				updateStatus(statusView)
			})
			time.Sleep(time.Second)
		}
	}()

	return statusView
}

func updateStatus(view *tview.TextView) {
	view.Clear()
	fmt.Fprintf(view, "Panago v%s\n", version)
	fmt.Fprintf(view, "Target IP: %s\n", status.ip)

	if status.ip == "" {
		fmt.Fprintf(view, "[yellow]Waiting for IP input...[white]\n")
		return
	}

	if status.detected {
		fmt.Fprintf(view, "[green]Player detected[white] - Version: %s\n", status.version)
		if status.exploited {
			fmt.Fprintf(view, "[red]Exploit active[white]\n")
		}
		fmt.Fprintf(view, "Last seen: %s\n", status.lastSeen.Format("15:04:05"))
	} else {
		fmt.Fprintf(view, "[red]Player not detected[white]\n")
	}
}

func createIPForm(content *tview.Flex) *tview.Form {
	form := tview.NewForm()
	form.AddInputField("IP Address", "", 20, nil, nil)
	form.AddButton("Connect", func() {
		ip := form.GetFormItemByLabel("IP Address").(*tview.InputField).GetText()
		if ip == "" {
			showMessage("Error", "Please enter an IP address")
			return
		}
		status.ip = ip

		// Start player detection
		go detectPlayer(ip)

		// Show main menu
		content.RemoveItem(form)
		menu := createMainMenu(content)
		content.AddItem(menu, 0, 1, true)
		app.SetFocus(menu)
	})
	form.AddButton("Quit", func() {
		app.Stop()
	})

	return form
}

func createMainMenu(content *tview.Flex) *tview.List {
	menu := tview.NewList().
		AddItem("Execute Exploit", "Run the initial exploit", 'e', func() {
			go executeExploit()
		}).
		AddItem("Create USB Disk", "Prepare a USB disk", 'u', func() {
			showDiskMenu()
		}).
		AddItem("Open Shell", "Open interactive shell", 's', func() {
			openShell()
		}).
		AddItem("Extract FPC Keys", "Dump firmware decryption keys", 'k', func() {
			go extractFPCKeys()
		}).
		AddItem("Backup Player", "Create player backup", 'b', func() {
			createBackup()
		}).
		AddItem("Update Player", "Install player update", 'p', func() {
			showUpdateMenu()
		}).
		AddItem("Back", "Return to IP input", 'q', func() {
			content.Clear()
			form := createIPForm(content)
			content.AddItem(form, 0, 1, true)
			app.SetFocus(form)
		})

	return menu
}

func detectPlayer(ip string) {
	for {
		if status.ip != ip {
			return // IP changed, stop detection
		}

		// Use discover package to check player
		info, err := discover.CheckPlayer(ip)
		if err == nil {
			status.detected = true
			status.version = info.Version
			status.lastSeen = time.Now()
		} else {
			status.detected = false
		}

		// Check exploit status
		if exploit.IsActive(ip) {
			status.exploited = true
		} else {
			status.exploited = false
		}

		time.Sleep(time.Second * 5)
	}
}

func executeExploit() {
	showProgress("Executing exploit...", func() error {
		punch := exploit.NewPunchExploit(status.ip)
		_, err := punch.Execute("SHELL")
		if err != nil {
			return fmt.Errorf("exploit failed: %w", err)
		}
		status.exploited = true
		return nil
	})
}

func openShell() {
	if !status.exploited {
		showMessage("Error", "Please execute exploit first")
		return
	}

	showMessage("Opening Shell", "Shell functionality will be implemented in terminal")
	// Implementation will launch external terminal with shell
}

func createBackup() {
	if !status.exploited {
		showMessage("Error", "Please execute exploit first")
		return
	}

	showProgress("Creating backup...", func() error {
		b, err := backup.New(status.ip)
		if err != nil {
			return err
		}
		defer b.Close()
		return b.CreateBackup()
	})
}

func showDiskMenu() {
	// Implementation for disk creation menu
}

func extractFPCKeys() {
	if !status.exploited {
		showMessage("Error", "Please execute exploit first")
		return
	}

	showProgress("Extracting FPC keys...", func() error {
		kd, err := keydump.New(status.ip)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer kd.Close()

		// First, download libfmupre.so from device
		showMessage("Info", "Downloading libfmupre.so from device...")
		
		// Patch it locally
		if err := keydump.PatchLibrary("libfmupre.so", "libfmupre_patched.so"); err != nil {
			return fmt.Errorf("failed to patch library: %w", err)
		}

		// Extract keys
		k1, k2, err := kd.ExtractKeys("libfmupre_patched.so")
		if err != nil {
			return err
		}

		// Display keys
		msg := fmt.Sprintf("FPC Keys extracted!\n\nK1: %x\nK2: %x\n\nSaved to fpc_keys.txt", k1, k2)
		showMessage("Success", msg)

		// Save to file
		ioutil.WriteFile("fpc_keys.txt", 
			[]byte(fmt.Sprintf("K1=%x\nK2=%x\n", k1, k2)), 0644)

		return nil
	})
}

func showUpdateMenu() {
	// Implementation for update menu
}

func showProgress(message string, operation func() error) {
	modal := tview.NewModal()
	modal.SetText(message)
	modal.SetBackgroundColor(tcell.ColorDefault)

	app.QueueUpdateDraw(func() {
		app.SetRoot(modal, false)
	})

	go func() {
		err := operation()
		app.QueueUpdateDraw(func() {
			if err != nil {
				showMessage("Error", err.Error())
			} else {
				showMessage("Success", "Operation completed")
			}
		})
	}()
}

func showMessage(title, message string) {
	modal := tview.NewModal()
	modal.SetText(message)
	modal.SetTitle(" " + title + " ")
	modal.AddButtons([]string{"OK"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		app.SetRoot(flex, true)
	})

	app.QueueUpdateDraw(func() {
		app.SetRoot(modal, false)
	})
}
