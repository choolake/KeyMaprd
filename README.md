# KeyMapr

> Remap your Logitech MX Master mouse buttons to any keyboard shortcut on macOS.  
> No Logitech Options+ required. Runs silently in the background. Configured via a simple JSON file.

---

## Features

- 🖱️ **Remap any extra mouse button** to any keyboard shortcut
- ⚡ **Lightweight daemon** — runs via launchd, auto-starts at login
- 🔄 **Hot-reload config** — edit your JSON and changes apply instantly, no restart needed
- 🔍 **`--dump` mode** — discover your button numbers by pressing each one
- 🚫 **No Logitech software needed** — works independently of Options+
- 📦 **Zero brew dependencies** — uses macOS-native IOKit and CoreGraphics directly

---

## Requirements

- macOS Ventura or later (Apple Silicon and Intel supported)
- **Accessibility** permission (prompted on first run)

---

## Installation

### Homebrew *(coming soon)*

```bash
brew tap choolake/keymaprd
brew install keymaprd
```

### From source

```bash
git clone https://github.com/choolake/keymaprd.git
cd keymaprd
go build -o keymaprd ./cmd/keymaprd/
codesign -s - -f --identifier com.choolake.keymaprd ./keymaprd
sudo cp keymaprd /usr/local/bin/keymaprd
```

> **Why codesign?** macOS tracks Accessibility permissions by binary identity. Re-signing with a stable identifier means you won't need to re-grant permission after every rebuild.

---

## Quick Start

### 1. Find your button numbers

Run the dump mode and press each button on your mouse:

```bash
keymaprd --dump
```

Output:
```
Button 3  DOWN  → config key: "btn3"
Button 4  DOWN  → config key: "btn4"
```

### 2. Create your config

```bash
mkdir -p ~/.config/keymaprd
cp /usr/local/share/keymaprd/config.example.json ~/.config/keymaprd/config.json
```

Edit `~/.config/keymaprd/config.json`:

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

### 3. Run it

```bash
keymaprd
```

### 4. Install as a background daemon (auto-start at login)

```bash
keymaprd install
```

To uninstall:

```bash
keymaprd uninstall
```

---

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
keymaprd                          Start the daemon (foreground)
keymaprd install                  Install as a launchd LaunchAgent (auto-start at login)
keymaprd uninstall                Remove the LaunchAgent
keymaprd --dump                   Print button events live (use to discover button numbers)
keymaprd --list-devices           List all connected HID devices
keymaprd --config <path>          Use a custom config file path
keymaprd --version                Print version
```

---

## Permissions

KeyMapr requires **Accessibility** permission to:
1. Read mouse button events (via CGEventTap)
2. Inject keyboard events (via CGEventPost)

macOS will prompt you automatically on first run. If it doesn't, go to:
**System Settings → Privacy & Security → Accessibility** and add your terminal (or `keymaprd` if running via launchd).

> **Note:** Input Monitoring permission is **not** required. KeyMapr uses CGEventTap in listen-only mode which only needs Accessibility.

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
