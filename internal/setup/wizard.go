//go:build darwin

// Package setup provides an interactive TUI wizard that runs on first launch
// when no config.json is found. It guides the user through:
//  1. Detecting which extra mouse buttons exist
//  2. Assigning a shortcut or named action to each button
//  3. Writing config.json to ~/.config/keymaprd/config.json
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/choolake/KeyMaprd/internal/eventtap"
	"github.com/choolake/KeyMaprd/internal/mapper"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// preset is a named shortcut shown in the assignment menu.
type preset struct {
	label  string
	action string // empty = "Custom..."
}

// presets is the list of common actions offered to the user.
var presets = []preset{
	{"Back                  (cmd+left)", "cmd+left"},
	{"Forward               (cmd+right)", "cmd+right"},
	{"Mission Control", "mission_control"},
	{"Launchpad", "launchpad"},
	{"Spotlight             (cmd+space)", "spotlight"},
	{"Screenshot            (cmd+shift+3)", "screenshot"},
	{"Copy                  (cmd+c)", "cmd+c"},
	{"Paste                 (cmd+v)", "cmd+v"},
	{"Undo                  (cmd+z)", "cmd+z"},
	{"Redo                  (cmd+shift+z)", "cmd+shift+z"},
	{"New Tab               (cmd+t)", "cmd+t"},
	{"Close Tab             (cmd+w)", "cmd+w"},
	{"Custom shortcut...", ""},
}

// wizard holds the full state of the setup wizard session.
type wizard struct {
	app        *tview.Application
	pages      *tview.Pages
	discovered []uint8          // button numbers the user pressed in order
	seen       map[uint8]bool   // dedup set for discovered
	mappings   map[uint8]string // button → action
	mu            sync.Mutex
	saved         bool
	installDaemon bool
	// detect page widgets — stored here so startDetection() can access them
	// after app.Run() has started (called from a key handler, not at build time).
	detectList   *tview.List
	detectStatus *tview.TextView
}

// Result is returned by Run() to tell the caller what actions the user requested.
type Result struct {
	Saved         bool // config was written successfully
	InstallDaemon bool // user wants to install as a login daemon
}

// Run launches the interactive setup wizard.
func Run() Result {
	w := &wizard{
		app:      tview.NewApplication(),
		pages:    tview.NewPages(),
		seen:     make(map[uint8]bool),
		mappings: make(map[uint8]string),
	}

	w.pages.AddPage("welcome", w.buildWelcomePage(), true, true)
	w.pages.AddPage("detect", w.buildDetectPage(), true, false)

	w.app.SetRoot(w.pages, true).EnableMouse(false)

	if err := w.app.Run(); err != nil {
		return Result{}
	}
	return Result{Saved: w.saved, InstallDaemon: w.installDaemon}
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Welcome
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildWelcomePage() tview.Primitive {
	const logo = ` _  __          __  __
| |/ /___ _   _|  \/  | __ _ _ __  _ __
| ' // _ \ | | | |\/| |/ _' | '_ \| '__|
| . \  __/ |_| | |  | | (_| | |_) | |
|_|\_\___|\__, |_|  |_|\__,_| .__/|_|
           |___/             |_|`

	body := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	fmt.Fprintf(body,
		"[green]%s[white]\n\n"+
			"[yellow]Welcome to KeyMapr Setup Wizard![white]\n\n"+
			"This wizard will help you map your extra mouse buttons\n"+
			"to keyboard shortcuts and system actions.\n\n"+
			"[::b]Requirements:[::]\n"+
			"  • Accessibility permission must be granted\n"+
			"    System Settings → Privacy & Security → Accessibility\n\n"+
			"[green][ S ][white] Start    [red][ Q ][white] Quit\n",
		logo,
	)

	body.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 's', 'S':
			w.pages.SwitchToPage("detect")
			w.app.SetFocus(w.pages)
			w.startDetection() // start eventtap NOW (app is running)
		case 'q', 'Q':
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 1, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" KeyMapr ").SetTitleColor(tcell.ColorYellow)
	return frame
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Button Detection
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildDetectPage() tview.Primitive {
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" Detected Buttons ").SetBorderColor(tcell.ColorGreen)

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetText(
			"[yellow]Step 1: Button Detection[white]\n\n" +
				"Press each extra button on your mouse [::b]now[::] (e.g. thumb buttons).\n" +
				"Each button number will appear in the list below.\n\n" +
				"[gray]Tip: skip left (btn0), right (btn1), and middle click (btn2).[white]\n" +
				"[gray]They are standard buttons and don't need mapping.[white]\n\n" +
				"[green][ Enter ][white] Continue to assignment    [red][ Q ][white] Quit",
		)

	status := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[gray]Starting button detection…[white]")

	// Store widgets so startDetection() can reach them after app.Run() begins.
	w.detectList = list
	w.detectStatus = status

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(instructions, 10, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(status, 3, 0, false)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			eventtap.Stop()
			w.mu.Lock()
			discovered := make([]uint8, len(w.discovered))
			copy(discovered, w.discovered)
			w.mu.Unlock()

			if len(discovered) == 0 {
				status.SetText("[red]No buttons detected yet. Press at least one button first.[white]")
				return nil
			}
			w.startAssignment(discovered, 0)
			return nil
		}
		switch event.Rune() {
		case 'q', 'Q':
			eventtap.Stop()
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(layout).SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" KeyMapr Setup — Step 1 of 3 ").SetTitleColor(tcell.ColorYellow)
	return frame
}

// ─────────────────────────────────────────────────────────────────────────────
// Detection goroutine (started AFTER app.Run(), i.e. from a key handler)
// ─────────────────────────────────────────────────────────────────────────────

// startDetection starts the CGEventTap and feeds events into the detect page list.
// Must be called after app.Run() has started so QueueUpdateDraw works correctly.
func (w *wizard) startDetection() {
	events, err := eventtap.Start()
	if err != nil {
		w.app.QueueUpdateDraw(func() {
			w.detectStatus.SetText(fmt.Sprintf(
				"[red]Could not start event tap: %v\n\n"+
					"Make sure Accessibility permission is granted:\n"+
					"System Settings → Privacy & Security → Accessibility → add keymaprd[white]",
				err,
			))
		})
		return
	}

	w.app.QueueUpdateDraw(func() {
		w.detectStatus.SetText("[green]Ready![white] Press each mouse button you want to map.")
	})

	go func() {
		for e := range events {
			if !e.Down {
				continue // only register presses, not releases
			}
			btn := e.Button // new variable per iteration — safe to capture in closure below

			w.mu.Lock()
			alreadySeen := w.seen[btn]
			if !alreadySeen {
				w.seen[btn] = true
				w.discovered = append(w.discovered, btn)
			}
			w.mu.Unlock()

			if !alreadySeen {
				w.app.QueueUpdateDraw(func() {
					w.detectList.AddItem(fmt.Sprintf("  btn%d  — ready to assign", btn), "", 0, nil)
					w.detectStatus.SetText(fmt.Sprintf(
						"[green]%d button(s) detected.[white] Keep pressing or press [green]Enter[white] to continue.",
						w.detectList.GetItemCount(),
					))
				})
			}
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Button Assignment (one per detected button)
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) startAssignment(buttons []uint8, idx int) {
	if idx >= len(buttons) {
		// All buttons assigned — move to preview/save
		w.pages.AddPage("save", w.buildSavePage(), true, false)
		w.pages.SwitchToPage("save")
		w.app.SetFocus(w.pages)
		return
	}

	btn := buttons[idx]
	pageID := fmt.Sprintf("assign-%d", idx)

	// Custom input form (shown when user picks "Custom shortcut...")
	customInput := tview.NewInputField().
		SetLabel("Enter shortcut: ").
		SetPlaceholder("e.g. cmd+shift+z").
		SetFieldWidth(30)
	customInput.SetBorder(true).
		SetTitle(" Custom Shortcut ").
		SetBorderColor(tcell.ColorYellow)
	customInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			action := customInput.GetText()
			if action != "" {
				w.mappings[btn] = action
				w.startAssignment(buttons, idx+1)
			}
		} else if key == tcell.KeyEscape {
			w.pages.SwitchToPage(pageID)
			w.app.SetFocus(w.pages)
		}
	})

	presetList := tview.NewList().ShowSecondaryText(false)
	for _, p := range presets {
		p := p // capture loop variable
		presetList.AddItem("  "+p.label, "", 0, func() {
			if p.action == "" {
				// Show custom input modal
				w.pages.AddPage("custom-input", center(customInput, 50, 5), true, false)
				w.pages.SwitchToPage("custom-input")
				w.app.SetFocus(customInput)
			} else {
				w.mappings[btn] = p.action
				w.startAssignment(buttons, idx+1)
			}
		})
	}

	header := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"[yellow]Step 2: Assign Actions  (%d of %d)[white]\n\n"+
				"[::b]Button %d[::] was detected.\n"+
				"Choose what it should do:\n\n"+
				"[gray]Use arrow keys to navigate, Enter to select.[white]",
			idx+1, len(buttons), btn,
		))

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 8, 0, false).
		AddItem(presetList, 0, 1, true)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Rune() == 'Q' {
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(layout).SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(fmt.Sprintf(" KeyMapr Setup — Assign btn%d ", btn)).
		SetTitleColor(tcell.ColorYellow)

	w.pages.AddPage(pageID, frame, true, false)
	w.pages.SwitchToPage(pageID)
	w.app.SetFocus(presetList)
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Save & Done
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildSavePage() tview.Primitive {
	configPath := mapper.DefaultConfigPath()

	// Build the config preview
	cfg := mapper.Config{Buttons: make(map[string]string)}
	for btn, action := range w.mappings {
		cfg.Buttons[fmt.Sprintf("btn%d", btn)] = action
	}

	preview, err := json.MarshalIndent(cfg, "", "  ")
	previewStr := string(preview)
	if err != nil {
		previewStr = fmt.Sprintf("error: %v", err)
	}

	body := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"[yellow]Step 3: Save Config[white]\n\n"+
				"[::b]Config will be saved to:[::]\n"+
				"  [green]%s[white]\n\n"+
				"[::b]Preview:[::]\n[cyan]%s[white]\n\n"+
				"[green][ S ][white] Save & Finish    [gray][ B ][white] Back    [red][ Q ][white] Quit",
			configPath, previewStr,
		))

	body.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 's', 'S':
			if err := writeConfig(configPath, cfg); err != nil {
				body.SetText(fmt.Sprintf("[red]Error saving config: %v\n\nPress Q to quit.[white]", err))
			} else {
				w.saved = true
				w.pages.AddPage("done", w.buildDonePage(configPath), true, false)
				w.pages.SwitchToPage("done")
				w.app.SetFocus(w.pages)
			}
		case 'b', 'B':
			// Go back to first assignment page
			w.startAssignment(w.discovered, 0)
		case 'q', 'Q':
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(body).SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" KeyMapr Setup — Step 3 of 3 ").SetTitleColor(tcell.ColorYellow)
	return frame
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Done
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildDonePage(configPath string) tview.Primitive {
	body := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf(
			"[green]✓ Config saved![white]  [::b]%s[::]\n\n"+
				"[yellow]One thing left — Accessibility permission:[white]\n"+
				"  System Settings → Privacy & Security → Accessibility\n"+
				"  Add [::b]keymaprd[::] to the list\n\n"+
				"[yellow]Auto-start at login?[white]\n\n"+
				"  [green][ Y ][white] Yes — install as LaunchAgent (runs on every login)\n"+
				"  [gray][ N ][white] No  — I'll run [cyan]keymaprd[white] manually\n\n"+
				"[gray]Tip: to re-run this wizard, delete config.json and run keymaprd again.[white]",
			configPath,
		))

	body.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'y', 'Y':
			w.installDaemon = true
			w.app.Stop()
		case 'n', 'N', 'q', 'Q':
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 2, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorGreen).
		SetTitle(" KeyMapr Setup — Complete! ").SetTitleColor(tcell.ColorGreen)
	return frame
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// writeConfig creates the config directory and writes config.json.
func writeConfig(path string, cfg mapper.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// center wraps a primitive in a centered flex layout — useful for modal overlays.
func center(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(p, height, 1, true).
				AddItem(nil, 0, 1, false),
			width, 1, true,
		).
		AddItem(nil, 0, 1, false)
}
