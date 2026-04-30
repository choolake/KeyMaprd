//go:build darwin

// Package setup provides an interactive TUI wizard for configuring KeyMapr.
// It uses a conversational one-button-at-a-time flow:
//   welcome → press a button → assign action → "got more?" → save → done
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	app           *tview.Application
	pages         *tview.Pages
	mappings      map[uint8]string // confirmed button → action
	existingCfg   *mapper.Config   // non-nil if a config already exists
	appendMode    bool             // true = merge new into existing on save
	saved         bool
	installDaemon bool
	lastBtn       uint8 // most recently detected button (for back navigation)
	// widgets that need to be accessible across callbacks
	waitStatus *tview.TextView
	saveBody   *tview.TextView
	savePath   string
}

// Result is returned by Run() to tell the caller what the user requested.
type Result struct {
	Saved         bool
	InstallDaemon bool
}

// Run launches the interactive setup wizard.
func Run() Result {
	w := &wizard{
		app:      tview.NewApplication(),
		pages:    tview.NewPages(),
		mappings: make(map[uint8]string),
	}

	// Load existing config so we can offer append/replace on save.
	if existing, err := mapper.LoadConfig(mapper.DefaultConfigPath()); err == nil {
		w.existingCfg = existing
		w.appendMode = true
	}

	w.pages.AddPage("welcome", w.buildWelcomePage(), true, true)
	w.pages.AddPage("waiting", w.buildWaitingPage(), true, false)

	// App-level input capture handles text-view pages (welcome, waiting, more,
	// save, done). Assign pages use focus-based dispatch (they contain a List).
	w.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		name, _ := w.pages.GetFrontPage()
		switch name {
		case "welcome":
			switch event.Rune() {
			case 's', 'S':
				w.pages.SwitchToPage("waiting")
				w.app.SetFocus(w.pages)
				w.startDetection()
				return nil
			case 'q', 'Q':
				w.app.Stop()
				return nil
			}
		case "waiting":
			switch event.Rune() {
			case 'b', 'B':
				eventtap.Stop()
				w.pages.SwitchToPage("welcome")
				w.app.SetFocus(w.pages)
				return nil
			case 'q', 'Q':
				eventtap.Stop()
				w.app.Stop()
				return nil
			}
		case "more":
			return w.handleMoreKey(event)
		case "save":
			return w.handleSaveKey(event)
		case "done":
			return w.handleDoneKey(event)
		}
		return event
	})

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

	existingNote := ""
	if w.existingCfg != nil {
		existingNote = fmt.Sprintf(
			"\n[gray]Psst — found an existing config with %d mapping(s).\nWe can add to it or start fresh, I'll ask at the end.[white]\n",
			len(w.existingCfg.Buttons),
		)
	}

	body := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	fmt.Fprintf(body,
		"[green]%s[white]\n\n"+
			"[yellow]Yo, open-sourcerers! 👋[white]\n\n"+
			"Your mouse has more buttons than your Netflix has good shows. 😂\n"+
			"Time to make those spare buttons [::b]actually do something[::]. 🔥\n\n"+
			"We go [::b]one button at a time[::] — press it, pick an action, done.\n"+
			"Easy money. Let's get this bread. 🍞\n"+
			"%s\n"+
			"[green][ S ][white] Let's gooo! 🚀    [red][ Q ][white] Nah, I'm good\n",
		logo, existingNote,
	)

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 1, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" KeyMapr — Mouse Button Hustle 🖱️ ").SetTitleColor(tcell.ColorYellow)
	return frame
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Waiting (single-button detection)
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildWaitingPage() tview.Primitive {
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(waitingIdleText)
	w.waitStatus = status

	hint := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[gray][ B ][white] Back    [red][ Q ][white] Quit")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(status, 5, 0, false).
		AddItem(nil, 0, 1, false).
		AddItem(hint, 1, 0, false)

	frame := tview.NewFrame(layout).SetBorders(2, 2, 2, 2, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" Press a Button! 🕹️ ").SetTitleColor(tcell.ColorYellow)
	return frame
}

const waitingIdleText = "[yellow]Aight, I'm listening... 👂[white]\n\n" +
	"Press that [::b]mystery button[::] on your mouse [::b]RIGHT NOW[::] 👇\n" +
	"[gray](Left & right click don't count — pick a side/thumb button.)[white]"

// startDetection starts the CGEventTap and waits for a single valid button press.
// Must be called from within the tview event loop.
func (w *wizard) startDetection() {
	// Reset status text each time detection restarts.
	w.waitStatus.SetText(waitingIdleText)

	events, err := eventtap.Start()
	if err != nil {
		w.waitStatus.SetText(fmt.Sprintf(
			"[red]Oof, can't start event tap: %v[white]\n\n"+
				"You need Accessibility permission first:\n"+
				"[::b]System Settings → Privacy & Security → Accessibility[::]\n"+
				"Add [cyan]keymaprd[white] to the list, then try again.",
			err,
		))
		return
	}

	go func() {
		for e := range events {
			if !e.Down {
				continue
			}
			if e.Button <= 1 {
				// Friendly nudge — keep listening.
				w.app.QueueUpdateDraw(func() {
					w.waitStatus.SetText(
						"[red]Psst — that's left/right click. Those are sacred. 🙏[white]\n\n" +
							"Gimme a [::b]side button[::] or thumb button — those hidden ones 👀\n" +
							"[gray](Left & right click don't count, superstar.)[white]",
					)
				})
				continue
			}
			// Got a valid button — stop tap and move to assignment.
			btn := e.Button
			eventtap.Stop()
			w.app.QueueUpdateDraw(func() {
				w.lastBtn = btn
				w.showAssign(btn)
			})
			return
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Assign (one button)
// ─────────────────────────────────────────────────────────────────────────────

// showAssign builds and displays the action-picker page for btn.
// Safe to call from QueueUpdateDraw (i.e. the event loop).
func (w *wizard) showAssign(btn uint8) {
	pageID := fmt.Sprintf("assign-%d", btn)

	customInput := tview.NewInputField().
		SetLabel("Shortcut: ").
		SetPlaceholder("e.g. cmd+shift+z").
		SetFieldWidth(30)
	customInput.SetBorder(true).
		SetTitle(" Custom Shortcut ✏️ ").
		SetBorderColor(tcell.ColorYellow)
	customInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if action := customInput.GetText(); action != "" {
				w.mappings[btn] = action
				w.showMore(btn, action)
			}
		} else if key == tcell.KeyEscape {
			w.pages.SwitchToPage(pageID)
			w.app.SetFocus(w.pages)
		}
	})

	presetList := tview.NewList().ShowSecondaryText(false)
	for _, p := range presets {
		p := p
		presetList.AddItem("  "+p.label, "", 0, func() {
			if p.action == "" {
				w.pages.AddPage("custom-input", center(customInput, 50, 5), true, false)
				w.pages.SwitchToPage("custom-input")
				w.app.SetFocus(customInput)
			} else {
				w.mappings[btn] = p.action
				w.showMore(btn, p.action)
			}
		})
	}

	header := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"[yellow]Ohhh I felt that! 👀[white]\n\n"+
				"That was [green][::b]btn%d[::][white]. Now give it a purpose in life — pick its destiny:\n\n"+
				"[gray]↑ ↓ navigate  Enter select  B re-detect  Q quit[white]",
			btn,
		))

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 6, 0, false).
		AddItem(presetList, 0, 1, true)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'b', 'B':
			// Remove this button's mapping (if any) and restart detection.
			delete(w.mappings, btn)
			w.pages.SwitchToPage("waiting")
			w.app.SetFocus(w.pages)
			w.startDetection()
			return nil
		case 'q', 'Q':
			w.app.Stop()
		}
		return event
	})

	frame := tview.NewFrame(layout).SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(fmt.Sprintf(" btn%d needs a job 🎯 ", btn)).
		SetTitleColor(tcell.ColorYellow)

	w.pages.AddPage(pageID, frame, true, false)
	w.pages.SwitchToPage(pageID)
	w.app.SetFocus(presetList)
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: More? (shown after each assignment)
// ─────────────────────────────────────────────────────────────────────────────

// showMore displays the "hell yeah, got more?" page after a button is assigned.
func (w *wizard) showMore(btn uint8, action string) {
	summary := ""
	for b, a := range w.mappings {
		tag := ""
		if b == btn {
			tag = "  [gray]← just added[white]"
		}
		summary += fmt.Sprintf("  [green]btn%d[white] → [cyan]%s[white]%s\n", b, a, tag)
	}

	body := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"[yellow]YESSS! 🔥[white]\n\n"+
				"[::b]btn%d[::] → [cyan]%s[white]  locked and loaded!\n\n"+
				"[::b]Your lineup so far:[::]\n%s\n"+
				"[green][ Y ][white] Add another button 👀\n"+
				"[green][ S ][white] Save & wrap this up 💾\n"+
				"[gray][ B ][white] Re-assign btn%d\n"+
				"[red][ Q ][white] Quit without saving",
			btn, action, summary, btn,
		))

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 2, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorGreen).
		SetTitle(" 🤘 Slappin'! ").SetTitleColor(tcell.ColorGreen)

	w.pages.AddPage("more", frame, true, false)
	w.pages.SwitchToPage("more")
	w.app.SetFocus(w.pages)
}

func (w *wizard) handleMoreKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 'y', 'Y':
		w.pages.SwitchToPage("waiting")
		w.app.SetFocus(w.pages)
		w.startDetection()
		return nil
	case 's', 'S':
		w.pages.AddPage("save", w.buildSavePage(), true, false)
		w.pages.SwitchToPage("save")
		w.app.SetFocus(w.pages)
		return nil
	case 'b', 'B':
		delete(w.mappings, w.lastBtn)
		w.showAssign(w.lastBtn)
		return nil
	case 'q', 'Q':
		w.app.Stop()
		return nil
	}
	return event
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Save
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildSavePage() tview.Primitive {
	configPath := mapper.DefaultConfigPath()
	w.savePath = configPath

	newCfg := mapper.Config{Buttons: make(map[string]string)}
	for btn, action := range w.mappings {
		newCfg.Buttons[fmt.Sprintf("btn%d", btn)] = action
	}

	var bodyText string
	if w.existingCfg != nil {
		existingJSON, _ := json.MarshalIndent(w.existingCfg, "", "  ")
		newJSON, _ := json.MarshalIndent(newCfg, "", "  ")
		modeLabel := "[red]Replace[white] — existing config gets yeeted 💀"
		if w.appendMode {
			modeLabel = "[green]Append[white] — merge new buttons into existing config 🤝"
		}
		bodyText = fmt.Sprintf(
			"[yellow]Almost there! 🏁[white]\n\n"+
				"[::b]Saving to:[::] [green]%s[white]\n\n"+
				"[::b]Existing config:[::]\n[gray]%s[white]\n\n"+
				"[::b]New buttons:[::]\n[cyan]%s[white]\n\n"+
				"Mode: %s\n"+
				"[gray][ A ][white] Toggle Append / Replace\n\n"+
				"[green][ S ][white] SAVE IT 💾    [gray][ B ][white] Back    [red][ Q ][white] Quit",
			configPath, string(existingJSON), string(newJSON), modeLabel,
		)
	} else {
		preview, _ := json.MarshalIndent(newCfg, "", "  ")
		bodyText = fmt.Sprintf(
			"[yellow]Almost there! 🏁[white]\n\n"+
				"[::b]Saving to:[::]\n  [green]%s[white]\n\n"+
				"[::b]Here's what we're locking in:[::]\n[cyan]%s[white]\n\n"+
				"[green][ S ][white] SAVE IT 💾    [gray][ B ][white] Back    [red][ Q ][white] Quit",
			configPath, string(preview),
		)
	}

	body := tview.NewTextView().SetDynamicColors(true).SetText(bodyText)
	w.saveBody = body

	frame := tview.NewFrame(body).SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).SetBorderColor(tcell.ColorMediumPurple).
		SetTitle(" Almost Done! 🏁 ").SetTitleColor(tcell.ColorYellow)
	return frame
}

func (w *wizard) handleSaveKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 's', 'S':
		newCfg := mapper.Config{Buttons: make(map[string]string)}
		for btn, action := range w.mappings {
			newCfg.Buttons[fmt.Sprintf("btn%d", btn)] = action
		}
		cfgToSave := newCfg
		if w.appendMode && w.existingCfg != nil {
			merged := mapper.Config{Buttons: make(map[string]string)}
			for k, v := range w.existingCfg.Buttons {
				merged.Buttons[k] = v
			}
			for k, v := range newCfg.Buttons {
				merged.Buttons[k] = v
			}
			cfgToSave = merged
		}
		if err := writeConfig(w.savePath, cfgToSave); err != nil {
			w.saveBody.SetText(fmt.Sprintf("[red]Oof, save failed: %v\n\nPress Q to rage-quit.[white]", err))
		} else {
			w.saved = true
			w.pages.AddPage("done", w.buildDonePage(w.savePath), true, false)
			w.pages.SwitchToPage("done")
			w.app.SetFocus(w.pages)
		}
		return nil
	case 'a', 'A':
		if w.existingCfg != nil {
			w.appendMode = !w.appendMode
			w.pages.RemovePage("save")
			w.pages.AddPage("save", w.buildSavePage(), true, false)
			w.pages.SwitchToPage("save")
			w.app.SetFocus(w.pages)
		}
		return nil
	case 'b', 'B':
		w.pages.SwitchToPage("more")
		w.app.SetFocus(w.pages)
		return nil
	case 'q', 'Q':
		w.app.Stop()
		return nil
	}
	return event
}

// ─────────────────────────────────────────────────────────────────────────────
// Page: Done
// ─────────────────────────────────────────────────────────────────────────────

func (w *wizard) buildDonePage(configPath string) tview.Primitive {
	body := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf(
			"[green]✅ Config saved![white]  [::b]%s[::]\n\n"+
				"[yellow]You're officially a power user now. 🏆[white]\n\n"+
				"One last thing — [::b]Accessibility permission[::] is required:\n"+
				"  System Settings → Privacy & Security → Accessibility\n"+
				"  Add [cyan]keymaprd[white] to the list\n\n"+
				"[yellow]Want keymaprd to fire up automatically at every login?[white]\n\n"+
				"  [green][ Y ][white] Heck yes — install as LaunchAgent 🚀\n"+
				"  [gray][ N ][white] Nah, I'll run [cyan]keymaprd[white] myself\n\n"+
				"[gray]To re-run this wizard: delete config.json and run keymaprd again[white]",
			configPath,
		))

	frame := tview.NewFrame(body).SetBorders(2, 2, 2, 2, 4, 4)
	frame.SetBorder(true).SetBorderColor(tcell.ColorGreen).
		SetTitle(" 🎉 You did it, legend! ").SetTitleColor(tcell.ColorGreen)
	return frame
}

func (w *wizard) handleDoneKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 'y', 'Y':
		w.installDaemon = true
		w.app.Stop()
		return nil
	case 'n', 'N', 'q', 'Q':
		w.app.Stop()
		return nil
	}
	return event
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

