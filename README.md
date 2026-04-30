# KeyMapr

> Remap your Logitech MX Master mouse buttons to any keyboard shortcut on macOS.  
> No Logitech Options+ required. Runs silently in the background.

---

## Features

- 🖱️ **Remap any extra mouse button** to any keyboard shortcut
- 🧙 **Interactive setup wizard** — launches on first run, no manual JSON editing needed
- ⚡ **Lightweight daemon** — runs via launchd, auto-starts at login
- 🔄 **Hot-reload config** — edit your JSON and changes apply instantly, no restart needed
- 🔍 **`--dump` mode** — discover your button numbers by pressing each one
- 🚫 **No Logitech software needed** — works independently of Options+
- 📦 **Zero brew dependencies** — uses macOS-native IOKit and CoreGraphics directly

---

## Requirements

- macOS Ventura or later (Apple Silicon and Intel supported)
- **Accessibility** + **Input Monitoring** permissions (prompted on first run)

---

## Installation

### Homebrew (recommended)

```bash
brew install choolake/tap/keymaprd
```

Then just run it — the wizard handles everything:

```bash
keymaprd
```

### From source

```bash
git clone https://github.com/choolake/KeyMaprd.git
cd KeyMaprd
go build -o keymaprd ./cmd/keymaprd/
codesign -s - -f --identifier com.choolake.keymaprd ./keymaprd
sudo cp keymaprd /usr/local/bin/keymaprd
```

> **Why codesign?** macOS tracks Accessibility permissions by binary identity. Re-signing with a stable identifier means you won't need to re-grant permission after every rebuild.

---

## Quick Start

### First run — interactive wizard

On first launch (when no config exists), the wizard starts automatically:

```bash
keymaprd
```

The wizard walks you through:
1. **Button detection** — press each mouse button you want to map
2. **Action assignment** — pick from a preset list or type a custom shortcut
3. **Save** — writes `~/.config/keymaprd/config.json`
4. **Auto-start** — optionally installs as a login daemon

To re-run the wizard at any time:

```bash
keymaprd setup
```

### Manual config (advanced)

If you prefer editing JSON directly, run `--dump` to find your button numbers:

```bash
keymaprd --dump
```

Output:
```
Button 3  DOWN  → config key: "btn3"
Button 4  DOWN  → config key: "btn4"
```

Then edit `~/.config/keymaprd/config.json`:

```json
{
  "buttons": {
    "btn3": "ctrl+left",
    "btn4": "ctrl+right",
    "btn5": "cmd+space",
    "btn6": "mission_control"
  }
}
```

Changes to this file are applied instantly — no restart required.

## Config Reference

Config file location: `~/.config/keymaprd/config.json`

### Supported modifiers

| Modifier | Symbol |
|----------|--------|
| `cmd`    | ⌘ Command |
| `shift`  | ⇧ Shift |
| `opt`    | ⌥ Option |
| `ctrl`   | ⌃ Control |

### Supported keys

| Category | Keys |
|----------|------|
| Letters  | `a` – `z` |
| Numbers  | `0` – `9` |
| Arrows   | `left` `right` `up` `down` |
| Special  | `space` `return` `tab` `escape` `delete` `backspace` |
| Function | `f1` – `f12` |
| Symbols  | `- = [ ] \ ; ' , . /` |

### Named actions

| Action | Effect |
|--------|--------|
| `mission_control` | Opens Mission Control |
| `launchpad`       | Opens Launchpad |
| `spotlight`       | Opens Spotlight (⌘Space) |
| `screenshot`      | Takes a screenshot (⌘⇧3) |

### Example config

```json
{
  "buttons": {
    "btn3": "ctrl+left",
    "btn4": "ctrl+right",
    "btn5": "cmd+space",
    "btn6": "mission_control",
    "btn7": "cmd+shift+4"
  }
}
```

Changes to this file are applied instantly — no restart required.

---

## CLI Reference

```
keymaprd                          Start (wizard auto-launches on first run)
keymaprd setup                    Re-run the interactive setup wizard
keymaprd install                  Install as a launchd LaunchAgent (auto-start at login)
keymaprd uninstall                Remove the LaunchAgent
keymaprd --dump                   Print button events live (use to discover button numbers)
keymaprd --list-devices           List all connected HID devices
keymaprd --config <path>          Use a custom config file path
keymaprd --version                Print version
```

---

## Permissions

KeyMapr requires **two** macOS privacy permissions:

| Permission | Why |
|---|---|
| **Accessibility** | Allows `CGEventTapCreate` to succeed and lets keymaprd inject keyboard events |
| **Input Monitoring** | Required by WindowServer before it delivers tap events to the process — without this, the tap creates fine but callbacks never fire (critical when running as a LaunchAgent) |

macOS will prompt you automatically on first run. If it doesn't, grant both manually:

- **System Settings → Privacy & Security → Accessibility** → add `keymaprd`
- **System Settings → Privacy & Security → Input Monitoring** → add `keymaprd`

> **Dev note:** Ad-hoc signing (`codesign -s -`) changes the binary hash on every rebuild, silently revoking both permissions. Re-grant them after each `go build`. A real Apple Developer certificate avoids this churn.

---

## Compatibility Note: Ctrl+Left / Ctrl+Right

On macOS Ventura and Sonoma, the Dock **ignores synthetic keyboard events** for space-switching (`Ctrl+←` / `Ctrl+→`), even from trusted processes. This is a deliberate Apple security hardening.

KeyMapr works around this automatically — when you map `ctrl+left` or `ctrl+right`, it routes those through the macOS Accessibility API (`osascript`) which the Dock trusts. All other shortcuts use the fast direct-injection path. The ~100–200ms latency for space-switching is imperceptible in practice.

---

## Logs

When running as a daemon (after `keymaprd install`):

```
~/Library/Logs/keymaprd/keymaprd.log        # stdout
~/Library/Logs/keymaprd/keymaprd.error.log  # stderr
```

---

## License

MIT
