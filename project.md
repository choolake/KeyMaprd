# KeyMapr

> **Learning Project:** Building this in Go as a learning exercise. Decisions are documented here so the codebase stays clear and intentional.

> **Working Agreement:** Before running any command, explain what it does and why. Wait for approval before executing.

An open-source macOS tool that reads Logitech MX Master mouse buttons and remaps them via a `config.json` file. Written in Go, distributed via Homebrew.

---

## Requirements

### Common Requirements
- Installable via `brew install`
- Read all MX Master mouse buttons and remap them to any keyboard shortcut or action
- Configured via `config.json`
- Runs as a background daemon (auto-starts on login)

### Sprint 1
- Read all mouse buttons and remap as per `config.json`
- Basic daemon mode with launchd support

---

## Architecture Decisions

### Language & Build
- **Language:** Go (learning project)
- **CGo:** Required for HID reading and event injection on macOS
- **Build constraint:** `CGO_ENABLED=1`, macOS only

### HID Reading
- **Library:** Native CGo IOKit (`IOHIDManagerCreate` / `IOHIDManagerCopyDevices`) for device enumeration (`--list-devices`)
  - No external library or `brew install hidapi` needed — uses macOS built-in IOKit framework directly
- **Button event capture:** `CGEventTap` (see below) — *not* raw HID device open
  - macOS Ventura/Sonoma blocks raw HID opens on Bluetooth mice (`0xE00002C1 privilege violation`)
  - Raw HID open approach was attempted and abandoned in favour of CGEventTap

### Button Event Capture (CGEventTap)
- **Method:** `CGEventTapCreate` via CGo (`internal/eventtap/tap.go`)
  - Registers a session-level event tap with `kCGEventTapOptionListenOnly`
  - Observes `kCGEventOtherMouseDown/Up` for extra buttons (btn3+)
  - Uses a **Unix pipe** as a CGo callback → Go channel bridge
  - Requires **Accessibility** + **Input Monitoring** permissions (see macOS Permissions below)
  - Works with Bluetooth mice on all modern macOS versions ✅
- **Frameworks:** `-framework CoreGraphics -framework CoreFoundation`

### Config Format
- File: `~/.config/keymaprd/config.json` (XDG-style user config)
- Example:
```json
{
  "buttons": {
    "0x10": "cmd+space",
    "0x14": "cmd+tab",
    "0xC0": "mission_control"
  }
}
```

### Background Daemon
- **Mechanism:** launchd plist (`/Library/LaunchDaemons/` for system-wide, root)
- **Label:** `com.keymaprd.daemon`
- **RunAtLoad + KeepAlive:** true
- **Logging:** `/usr/local/var/log/keymaprd.log`

### Event Injection
- **Method:** CGo with `CoreGraphics.framework` (`CGEventPost`)
  - Use `CGEventCreateKeyboardEvent` for key presses
  - Use `CGEventPost(kCGHIDEventTap, event)` to inject — **not** `kCGSessionEventTap`
- **Frameworks:** `-framework CoreGraphics -framework CoreFoundation`
- **Why `kCGHIDEventTap`:**

| | `kCGSessionEventTap` | `kCGHIDEventTap` |
|---|---|---|
| Level | Application session | Hardware (HID) level |
| System shortcuts (Spaces, Spotlight, Mission Control) | ❌ Ignored | ✅ Triggered |
| Behaves like real keyboard | ❌ | ✅ |
| Needs root? | No | No (Accessibility permission sufficient) |

  `kCGHIDEventTap` makes the injected event indistinguishable from a real physical keypress — required for system-level shortcuts like `Cmd+Space` (Spotlight) to fire correctly.

- **`ctrl+left` / `ctrl+right` space-switching exception:**
  - `CGEventPost` **cannot** trigger Dock space-switching on macOS Ventura/Sonoma, even at `kCGHIDEventTap` and even as root — the Dock verifies event origin and rejects synthetic ctrl events
  - Fix: detect `ctrl+left` / `ctrl+right` in `Press()` and shell out to `osascript`:
    ```
    osascript -e 'tell application "System Events" to key code 123 using {control down}'
    ```
  - This goes through the Accessibility API which the Dock trusts
  - All other combos still use the fast CGEventPost path
  - ~100-200ms latency is acceptable for space-switching

- **Navigation key flag (`kCGEventFlagMaskNumericPad` = `0x00200000`):**
  - Arrow keys (`left`, `right`, `up`, `down`) and navigation keys (`delete`, `return`) **must** have this flag set
  - Real keyboards always set it automatically — without it, `Ctrl+Left` injects as ANSI escape `^[[1;5C` (raw terminal character) instead of triggering the space-switch shortcut
  - Fix: detect navigation keys and OR in `flagNumPad` automatically before posting

### macOS Permissions
- **Accessibility** — required for `CGEventTapCreate` to succeed and for `CGEventPost` to inject events
- **Input Monitoring** — required by WindowServer to actually deliver tap events to the callback; without it the tap creates fine but callbacks never fire (critical when running as a LaunchAgent on macOS Ventura/Sonoma)
  - Note: the `kCGEventTapOptionListenOnly` flag was expected to exempt us from Input Monitoring per Apple docs, but in practice macOS 13+ checks `kTCCServiceListenEvent` before delivering events to background agents regardless
- Running the binary from a terminal: the terminal app also needs Accessibility permission
- Binary must be codesigned with a stable identifier for TCC to track it:
  ```bash
  codesign -s - -f --identifier com.choolake.keymaprd ./keymaprd
  ```

> **Note:** Raw HID device open (`go-hid OpenPath`) was attempted but blocked by macOS on Bluetooth HID devices with error `0xE00002C1 (privilege violation)` even with Input Monitoring granted. CGEventTap is the correct macOS-native approach.

### Distribution (Homebrew)
- Repository: `github.com/choolake/homebrew-keymaprd` (Homebrew tap)
- Go module: `github.com/choolake/keymaprd`
- Formula declares `depends_on "go" => :build` (no `hidapi` needed — IOKit is built into macOS)
- Pre-built bottles via GitHub Actions (avoids requiring users to have Go installed)
- Install: `brew tap choolake/keymaprd && brew install keymaprd`

---

### Project Structure

```
KeyMapr/
├── cmd/
│   └── keymaprd/
│       ├── main.go              # Entry point: --dump, --list-devices, --config, install, uninstall, start
│       └── launchd.go           # install/uninstall subcommands — writes LaunchAgent plist
├── internal/
│   ├── hid/
│   │   └── reader.go            # Device enumeration (native IOKit CGo, --list-devices)
│   ├── eventtap/
│   │   └── tap.go               # CGEventTap: mouse button capture (CGo + CoreGraphics)
│   ├── mapper/
│   │   ├── config.go            # Config struct + JSON loader
│   │   ├── mapper.go            # Button number → action lookup
│   │   └── watcher.go           # fsnotify hot-reload wrapper
│   └── inject/
│       └── keyboard.go          # CGo CoreGraphics event injection (CGEventPost + osascript)
├── config.example.json          # Example mapping: { "buttons": { "btn3": "cmd+space" } }
├── go.mod                       # Module: github.com/choolake/keymaprd
├── go.sum
├── Formula/
│   └── keymaprd.rb              # Homebrew formula
├── .github/
│   └── workflows/
│       └── release.yml          # GitHub Actions: build arm64 + amd64 binaries on tag push
└── project.md                   # This file — living document
```

---

## Decisions

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Which PIDs to support? | Both USB (`0xC548`) + Bluetooth (`0xB023`) | Enumeration by VID `0x046D` costs nothing extra |
| 2 | HID++ for gesture/top buttons? | **Defer to Sprint 2** | Validate core loop first; add HID++ 2.0 parser later |
| 3 | Config hot-reload? | **Defer to Sprint 2** | Keep Sprint 1 focused; add `fsnotify` in Sprint 2 |
| 4 | Logitech Options+ conflict? | **Document warning only** | Recommend users quit Options+; not worth engineering around |
| 5 | Root daemon vs user-level agent? | **User-level LaunchAgent** (`~/Library/LaunchAgents/`) | No `sudo` needed; friendlier for Homebrew distribution |
| 6 | `--list-devices` diagnostic? | **Yes, in Sprint 1** | Trivial to add via `hid.Enumerate`; essential for debugging |

---

## Key Learning Topics (Go)

Concepts this project will teach, in order of encounter:
1. **Go modules** — `go mod init`, `go.mod`, `go.sum`
2. **Package structure** — `cmd/` vs `internal/` conventions
3. **CGo** — calling C code from Go, `#cgo` directives, memory management
4. **Goroutines & channels** — concurrent HID read loop + event dispatcher
5. **JSON config** — `encoding/json`, struct tags, file I/O
6. **OS signals** — graceful shutdown (`os/signal`, `syscall.SIGTERM`)
7. **Build tags** — `//go:build darwin` for macOS-only code
8. **Interfaces** — e.g., `ButtonReader` interface for testability
9. **Error handling** — idiomatic Go error wrapping (`fmt.Errorf`, `errors.Is`)
10. **Testing** — table-driven tests for the mapper logic

---

## Sprints

### Sprint 1 — Core Remapping ✅ Complete
- [x] Initialize Go module (`go mod init github.com/choolake/keymaprd`)
- [x] Set up project directory structure
- [x] `--list-devices` — enumerate HID devices via go-hid
- [x] `--dump` — print live mouse button events via CGEventTap
- [x] Config loader (`~/.config/keymaprd/config.json`)
- [x] Mapper: button number (`btn3`) → action string (`cmd+space`)
- [x] Event injection via CGo + CoreGraphics `CGEventPost`
- [x] CGEventTap event capture (`internal/eventtap/tap.go`)
- [x] Basic CLI: `keymaprd`, `--config`, `--dump`, `--list-devices`, `--version`
- [x] Ad-hoc codesign with stable identifier (`com.choolake.keymaprd`)

### Sprint 2 — Daemon & Distribution (In Progress)
- [x] `--config` falls back to default path when no value is provided
- [x] Comprehensive `config.example.json` — all keys, modifiers, named actions documented
- [x] Replace `go-hid` with native CGo IOKit enumeration for `--list-devices`
      — removed `github.com/sstallion/go-hid` and `brew install hidapi` dependency entirely
      — uses `IOHIDManagerCreate` + `IOHIDManagerCopyDevices` directly in C
- [x] launchd plist + install/uninstall commands
      — `keymaprd install` writes `~/Library/LaunchAgents/com.choolake.keymaprd.plist` and loads it
      — `keymaprd uninstall` unloads and removes the plist
      — Logs to `~/Library/Logs/keymaprd/`
- [x] Homebrew tap + formula (`Formula/keymaprd.rb`)
      — `depends_on "go" => :build` only; no hidapi or other brew deps
- [x] Pre-built GitHub Actions bottles (`.github/workflows/release.yml`)
      — builds arm64 + amd64 binaries on every `v*.*.*` tag push
      — uploads to GitHub Release automatically

### Sprint 3 — Make User Friendly 
- [x] Investigate the ways to make the installtion frictionless and impliment the quick wins
      — `brew install choolake/tap/keymaprd` is now a single-line install
      — wizard auto-launches on first run (no manual config copy)
      — wizard done page offers Y/N to install as LaunchAgent (calls `keymaprd install`)
      — `keymaprd setup` subcommand re-runs wizard at any time
      — simplified brew caveats: just says "run keymaprd"
- [x] Fun TUI config editor — `internal/setup/wizard.go` using `tview`
      — launches automatically on first run when `~/.config/keymaprd/config.json` is missing
      — Step 1: live button detection (press each button, they appear in the list)
      — Step 2: per-button action picker (presets + custom shortcut input)
      — Step 3: config preview → save to `~/.config/keymaprd/config.json`
      — after wizard, keymaprd starts normally without restart needed
- [x] bug: /opt/homebrew/share/keymaprd/config.example.json not available after brew install
      — fixed formula: added `(share/"keymaprd").install "config.example.json"`

### Sprint 4 — Advanced Mappings (Future)
- [ ] bug: In the setup when you add mission control it appeard empty at the save -- I suspect it's same for other named commands that supports for config
- [ ] Update the --help to display all the commands supported and support ? and other standard help 
- [ ] When using setup it just add only the required entries that remove the other example config in the exmaple and this removes the ability to manualy edit the file
- [ ] Permission check and prompt if not being granted. Evaluate options and impliment an one.
- [ ] **HID++ 2.0 protocol parser for gesture/top button (btn5)**
      — btn5 is a Logitech proprietary gesture button, NOT a standard HID button
      — it sends HID++ 2.0 feature reports, not `Button usage page (0x0009)` events
      — neither CGEventTap nor IOHIDManager input callbacks can see it
      — fix requires: open raw HID device, send `HIDPP_GET_FEATURE` requests,
        enable `GestureButtonControl` feature (0x2150), parse incoming feature reports
      — this is what LogiOps (Linux) and Options+ (macOS) do internally


### Sprint 5 — Nice to have Mappings (Future)
- [ ] App-specific profiles (different mappings per frontmost app)
- [ ] Mouse event actions (not just keyboard)




