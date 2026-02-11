package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/tridentsx/panago/internal/backup"
	"github.com/tridentsx/panago/internal/discover"
	"github.com/tridentsx/panago/internal/disk"
	"github.com/tridentsx/panago/internal/exploit"
	"github.com/tridentsx/panago/internal/keydump"
	"github.com/tridentsx/panago/internal/upnp"
)

// Build-time variable via -ldflags (optional)
var version = "dev"

var (
	app     *tview.Application
	status  *PlayerStatus
	flex    *tview.Flex
	content *tview.Flex
)

type PlayerStatus struct {
	ip        string
	detected  bool
	version   string
	exploited bool
	lastSeen  time.Time
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
	content = tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(content, 0, 1, true)

	// Show discovery screen instead of IP form
	showDiscoveryScreen()

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
		fmt.Fprintf(view, "[yellow]Waiting for player selection...[white]\n")
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

func showDiscoveryScreen() {
	content.Clear()

	// Create a flex to hold scanning status + list
	scanFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	statusText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	statusText.SetText("[yellow]Scanning for Panasonic players...[white]")

	deviceList := tview.NewList()
	deviceList.SetBorder(true).SetTitle(" Discovered Players ")

	scanFlex.AddItem(statusText, 3, 0, false)
	scanFlex.AddItem(deviceList, 0, 1, true)

	content.AddItem(scanFlex, 0, 1, true)
	app.SetFocus(deviceList)

	// Run discovery in background
	go func() {
		devices, err := upnp.DiscoverPanasonic(3)

		app.QueueUpdateDraw(func() {
			deviceList.Clear()

			if err != nil {
				statusText.SetText(fmt.Sprintf("[red]Discovery error: %s[white]", err.Error()))
			} else if len(devices) == 0 {
				statusText.SetText("[yellow]No Panasonic players found.[white]")
			} else {
				statusText.SetText(fmt.Sprintf("[green]Found %d player(s)[white]", len(devices)))
			}

			// Add discovered devices
			for _, dev := range devices {
				ip := upnp.ExtractIP(dev.Location)
				label := fmt.Sprintf("%s (%s)", dev.FriendlyName, dev.ModelName)
				desc := fmt.Sprintf("IP: %s", ip)
				capturedIP := ip
				deviceList.AddItem(label, desc, 0, func() {
					selectPlayer(capturedIP)
				})
			}

			// Add manual IP entry option
			deviceList.AddItem("Manual IP Entry", "Enter IP address manually", 'm', func() {
				showManualIPForm()
			})

			// Add rescan option
			deviceList.AddItem("Rescan", "Search for players again", 'r', func() {
				showDiscoveryScreen()
			})

			// Add quit option
			deviceList.AddItem("Quit", "Exit application", 'q', func() {
				app.Stop()
			})
		})
	}()
}

func showManualIPForm() {
	content.Clear()

	form := tview.NewForm()
	form.AddInputField("IP Address", "", 20, nil, nil)
	form.AddButton("Connect", func() {
		ip := form.GetFormItemByLabel("IP Address").(*tview.InputField).GetText()
		if ip == "" {
			showMessage("Error", "Please enter an IP address")
			return
		}
		selectPlayer(ip)
	})
	form.AddButton("Back", func() {
		showDiscoveryScreen()
	})

	content.AddItem(form, 0, 1, true)
	app.SetFocus(form)
}

func selectPlayer(ip string) {
	status.ip = ip
	go detectPlayer(ip)
	showMainMenu()
}

func showMainMenu() {
	content.Clear()
	menu := createMainMenu()
	content.AddItem(menu, 0, 1, true)
	app.SetFocus(menu)
}

func createMainMenu() *tview.List {
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
		AddItem("Back", "Return to player selection", 'q', func() {
			status.ip = ""
			status.detected = false
			status.exploited = false
			showDiscoveryScreen()
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
	content.Clear()

	statusText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	statusText.SetText("[yellow]Listing USB disks...[white]")

	diskList := tview.NewList()
	diskList.SetBorder(true).SetTitle(" USB Disks ")

	diskFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	diskFlex.AddItem(statusText, 3, 0, false)
	diskFlex.AddItem(diskList, 0, 1, true)

	content.AddItem(diskFlex, 0, 1, true)
	app.SetFocus(diskList)

	go func() {
		mgr := disk.NewDiskManager()
		disks, err := mgr.ListUSBDisks()

		app.QueueUpdateDraw(func() {
			diskList.Clear()

			if err != nil {
				statusText.SetText(fmt.Sprintf("[red]Error: %s[white]", err.Error()))
			} else if len(disks) == 0 {
				statusText.SetText("[yellow]No USB disks found.[white]")
			} else {
				statusText.SetText(fmt.Sprintf("[green]Found %d USB disk(s)[white]", len(disks)))
			}

			for _, d := range disks {
				label := fmt.Sprintf("%s (%s) - %s", d.Name, d.Model, d.Size)
				desc := d.DevicePath
				capturedDisk := d
				diskList.AddItem(label, desc, 0, func() {
					confirmDiskWrite(capturedDisk)
				})
			}

			diskList.AddItem("Refresh", "Scan for disks again", 'r', func() {
				showDiskMenu()
			})

			diskList.AddItem("Back", "Return to main menu", 'q', func() {
				showMainMenu()
			})
		})
	}()
}

func confirmDiskWrite(d disk.Disk) {
	modal := tview.NewModal()
	modal.SetText(fmt.Sprintf("WARNING: All data on %s (%s) will be erased.\n\nContinue?", d.Name, d.DevicePath))
	modal.AddButtons([]string{"Yes", "No"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonLabel == "Yes" {
			executeDiskWrite(d)
		} else {
			app.SetRoot(flex, true)
			showDiskMenu()
		}
	})

	app.SetRoot(modal, false)
}

func executeDiskWrite(d disk.Disk) {
	app.SetRoot(flex, true)
	content.Clear()

	progressView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	progressView.SetBorder(true).SetTitle(" Disk Write Progress ")

	content.AddItem(progressView, 0, 1, false)

	go func() {
		mgr := disk.NewDiskManager()

		addLine := func(text string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(progressView, "%s\n", text)
				progressView.ScrollToEnd()
			})
		}

		addLine("[yellow]Formatting disk...[white]")
		if err := mgr.Format(d); err != nil {
			addLine(fmt.Sprintf("[red]Format failed: %s[white]", err.Error()))
			addLine("\nPress any key to return.")
			app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				app.SetInputCapture(nil)
				app.QueueUpdateDraw(func() { showMainMenu() })
				return nil
			})
			return
		}
		addLine("[green]Format complete[white]")

		addLine("[yellow]Writing image...[white]")
		progress := func(msg string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(progressView, "\r%s", msg)
			})
		}

		imageFile := "res/drive.img.gz"
		if err := mgr.WriteImage(d, imageFile, progress); err != nil {
			addLine(fmt.Sprintf("\n[red]Write failed: %s[white]", err.Error()))
			addLine("\nPress any key to return.")
			app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				app.SetInputCapture(nil)
				app.QueueUpdateDraw(func() { showMainMenu() })
				return nil
			})
			return
		}

		addLine("\n[green]Disk write complete![white]")
		addLine("\nPress any key to return.")
		app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			app.SetInputCapture(nil)
			app.QueueUpdateDraw(func() { showMainMenu() })
			return nil
		})
	}()
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
		os.WriteFile("fpc_keys.txt",
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
