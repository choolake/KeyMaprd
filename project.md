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
- **Library:** `github.com/sstallion/go-hid` — used for **device enumeration only** (`--list-devices`)
  - Requires `hidapi` as a system dependency (`brew install hidapi`)
- **Button event capture:** `CGEventTap` (see below) — *not* raw HID device open
  - macOS Ventura/Sonoma blocks raw HID opens on Bluetooth mice (`0xE00002C1 privilege violation`)
  - Raw HID open approach was attempted and abandoned in favour of CGEventTap

### Button Event Capture (CGEventTap)
- **Method:** `CGEventTapCreate` via CGo (`internal/eventtap/tap.go`)
  - Registers a session-level event tap with `kCGEventTapOptionListenOnly`
  - Observes `kCGEventOtherMouseDown/Up` for extra buttons (btn3+)
  - Uses a **Unix pipe** as a CGo callback → Go channel bridge
  - Requires **Accessibility** permission only (no Input Monitoring needed)
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

  `kCGHIDEventTap` makes the injected event indistinguishable from a real physical keypress — required for system-level shortcuts like `Ctrl+Left` (switch spaces) to fire correctly.

- **Navigation key flag (`kCGEventFlagMaskNumericPad` = `0x00200000`):**
  - Arrow keys (`left`, `right`, `up`, `down`) and navigation keys (`delete`, `return`) **must** have this flag set
  - Real keyboards always set it automatically — without it, `Ctrl+Left` injects as ANSI escape `^[[1;5C` (raw terminal character) instead of triggering the space-switch shortcut
  - Fix: detect navigation keys and OR in `flagNumPad` automatically before posting

### macOS Permissions
- **Accessibility** — required for both CGEventTap (read) and CGEventPost (inject)
- **Input Monitoring** — NOT required (CGEventTap with `ListenOnly` avoids this)
- Running the binary from a terminal: the terminal app also needs Accessibility permission
- Binary must be codesigned with a stable identifier for TCC to track it:
  ```bash
  codesign -s - -f --identifier com.choolake.keymaprd ./keymaprd
  ```

> **Note:** Raw HID device open (`go-hid OpenPath`) was attempted but blocked by macOS on Bluetooth HID devices with error `0xE00002C1 (privilege violation)` even with Input Monitoring granted. CGEventTap is the correct macOS-native approach.

### Distribution (Homebrew)
- Repository: `github.com/choolake/homebrew-keymaprd` (Homebrew tap)
- Go module: `github.com/choolake/keymaprd`
- Formula declares `depends_on "hidapi"` and `depends_on "go" => :build`
- Pre-built bottles via GitHub Actions (avoids requiring users to have Go installed)
- Install: `brew tap choolake/keymaprd && brew install keymaprd`

---

## Project Structure

```
KeyMapr/
├── cmd/
│   └── keymaprd/
│       └── main.go              # Entry point: --dump, --list-devices, --config, start
├── internal/
│   ├── hid/
│   │   └── reader.go            # Device enumeration only (go-hid, --list-devices)
│   ├── eventtap/
│   │   └── tap.go               # CGEventTap: mouse button capture (CGo + CoreGraphics)
│   ├── mapper/
│   │   ├── config.go            # Config struct + JSON loader
│   │   └── mapper.go            # Button number → action lookup
│   └── inject/
│       └── keyboard.go          # CGo CoreGraphics event injection (CGEventPost)
├── config.example.json          # Example mapping: { "buttons": { "btn3": "cmd+space" } }
├── go.mod                       # Module: github.com/choolake/keymaprd
├── go.sum
├── Formula/
│   └── keymaprd.rb              # Homebrew formula (Sprint 2)
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
- [ ] Replace `go-hid` with native CGo IOKit enumeration for `--list-devices`
      — removes `github.com/sstallion/go-hid` and `brew install hidapi` dependency entirely
      — use `IOHIDManagerCreate` + `IOHIDManagerCopyDevices` directly in C
- [ ] launchd plist + install/uninstall commands
- [ ] Homebrew tap + formula
- [ ] Bundle a default `config.json` in the Homebrew installation (works out of the box)
- [x] Config hot-reload with `fsnotify` (`internal/mapper/watcher.go`)
- [ ] Pre-built GitHub Actions bottles

### Sprint 3 — Advanced Mappings (Future)
- [ ] HID++ 2.0 protocol parser for gesture/top buttons
- [ ] Mouse event actions (not just keyboard)
- [ ] App-specific profiles (different mappings per frontmost app)
- [ ] GUI config editor (optional)




