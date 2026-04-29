//go:build darwin

package inject

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CGEvent.h>
#include <CoreGraphics/CGEventTypes.h>
#include <CoreGraphics/CGEventSource.h>
#include <CoreFoundation/CoreFoundation.h>

// postKey posts a regular key down or key up event with the given flags.
// Used for non-modifier keys (letters, arrows, function keys, etc.).
void postKey(int keyCode, int down, uint64_t flags) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
    CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)keyCode, down);
    CGEventSetFlags(e, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    if (src) CFRelease(src);
}

// postModifier sends a kCGEventFlagsChanged event for a modifier key.
//
// Real keyboards NEVER send kCGEventKeyDown/Up for modifier keys — they always
// send kCGEventFlagsChanged. The Dock (which handles ctrl+arrow for space
// switching) and WindowServer both specifically watch for kCGEventFlagsChanged
// to track the modifier state. Sending a regular kCGEventKeyDown for ctrl is
// ignored by the Dock, which is why ctrl+left never switched spaces.
//
// flags should include the modifier's own flag when pressing (down=1) and
// omit it when releasing (down=0), plus any other already-held modifiers.
void postModifier(int keyCode, int down, uint64_t flags) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
    CGEventRef e = CGEventCreate(src);
    CGEventSetType(e, kCGEventFlagsChanged);
    CGEventSetIntegerValueField(e, kCGKeyboardEventKeycode, (int64_t)keyCode);
    CGEventSetFlags(e, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    if (src) CFRelease(src);
}
*/
import "C"

import (
	"fmt"
	"os/exec"
	"strings"
)

// appLaunchActions maps named actions to their app path (launched via `open`).
// Using `open` instead of osascript avoids Accessibility permission issues when
// running as a LaunchAgent — `open` goes through Launch Services which works
// in any execution context.
var appLaunchActions = map[string]string{
	"mission_control": "/System/Applications/Mission Control.app",
	"launchpad":       "", // no standalone app — inject F4 at HID level instead
}

// CGEventFlags modifier bitmasks (from CoreGraphics/CGEventTypes.h).
const (
	flagCommand = 0x00100000
	flagShift   = 0x00020000
	flagOption  = 0x00080000
	flagControl = 0x00040000
	flagNumPad  = 0x00200000 // must be set for arrow/navigation keys
)

// kVK_Control is the Carbon virtual key code for the left Control key.
// Required when sending kCGEventFlagsChanged for ctrl modifiers.
const kVK_Control = 0x3B

// navigationKeys is the set of keys that require flagNumPad to be set.
// macOS real keyboards always set this flag for these keys — without it
// system shortcuts (e.g. Ctrl+Left for spaces) don't fire correctly.
var navigationKeys = map[string]bool{
	"left": true, "right": true, "up": true, "down": true,
	"delete": true, "backspace": true,
	"return": true, "enter": true,
}

// keyCodes maps human-readable key names to macOS CGKeyCode values.
// These come from Carbon/Events.h (kVK_* constants).
var keyCodes = map[string]int{
	// Letters
	"a": 0x00, "s": 0x01, "d": 0x02, "f": 0x03,
	"h": 0x04, "g": 0x05, "z": 0x06, "x": 0x07,
	"c": 0x08, "v": 0x09, "b": 0x0B, "q": 0x0C,
	"w": 0x0D, "e": 0x0E, "r": 0x0F, "y": 0x10,
	"t": 0x11, "1": 0x12, "2": 0x13, "3": 0x14,
	"4": 0x15, "6": 0x16, "5": 0x17, "=": 0x18,
	"9": 0x19, "7": 0x1A, "-": 0x1B, "8": 0x1C,
	"0": 0x1D, "]": 0x1E, "o": 0x1F, "u": 0x20,
	"[": 0x21, "i": 0x22, "p": 0x23, "l": 0x25,
	"j": 0x26, "'": 0x27, "k": 0x28, ";": 0x29,
	"\\": 0x2A, ",": 0x2B, "/": 0x2C, "n": 0x2D,
	"m": 0x2E, ".": 0x2F,
	// Special keys
	"return": 0x24, "enter": 0x24,
	"tab":    0x30,
	"space":  0x31,
	"delete": 0x33, "backspace": 0x33,
	"escape": 0x35,
	"left":   0x7B, "right": 0x7C, "down": 0x7D, "up": 0x7E,
	// Function keys
	"f1": 0x7A, "f2": 0x78, "f3": 0x63, "f4": 0x76,
	"f5": 0x60, "f6": 0x61, "f7": 0x62, "f8": 0x64,
	"f9": 0x65, "f10": 0x6D, "f11": 0x67, "f12": 0x6F,
}

// namedActions maps special action names to their default macOS keyboard shortcuts.
var namedActions = map[string]string{
	"spotlight":  "cmd+space",
	"screenshot": "cmd+shift+3",
}

// Press parses an action string and injects the corresponding key events.
// Supported formats:
//   - "space"              — single key
//   - "cmd+space"          — key with modifiers (cmd, shift, option/opt, ctrl)
//   - "cmd+shift+3"        — multiple modifiers
//   - "mission_control"    — named action (launches via open or HID injection)
func Press(action string) error {
	// Resolve named shortcuts first.
	if resolved, ok := namedActions[action]; ok {
		action = resolved
	}

	normalized := strings.ToLower(action)

	// Named app/system actions: launch via `open` or inject F-key at HID level.
	// We do NOT use osascript/System Events here because those require Accessibility
	// for the osascript process itself, which fails when running as a LaunchAgent.
	if appPath, ok := appLaunchActions[normalized]; ok {
		if appPath != "" {
			// Launch app directly via Launch Services — works from any context.
			return exec.Command("open", appPath).Run()
		}
		// launchpad: inject F4 at kCGHIDEventTap — the Dock intercepts it.
		C.postKey(C.int(keyCodes["f4"]), C.int(1), 0)
		C.postKey(C.int(keyCodes["f4"]), C.int(0), 0)
		return nil
	}

	parts := strings.Split(normalized, "+")
	if len(parts) == 0 {
		return fmt.Errorf("empty action")
	}

	// The last part is the key; everything before it is a modifier.
	keyName := parts[len(parts)-1]
	modifiers := parts[:len(parts)-1]

	keyCode, ok := keyCodes[keyName]
	if !ok {
		return fmt.Errorf("unknown key %q in action %q", keyName, action)
	}

	var flags uint64
	hasCtrl := false
	for _, mod := range modifiers {
		switch mod {
		case "cmd", "command":
			flags |= flagCommand
		case "shift":
			flags |= flagShift
		case "opt", "option", "alt":
			flags |= flagOption
		case "ctrl", "control":
			flags |= flagControl
			hasCtrl = true
		default:
			return fmt.Errorf("unknown modifier %q in action %q", mod, action)
		}
	}

	// Arrow and navigation keys always need the NumPad flag set, just like
	// a real keyboard. Without this, Ctrl+Left won't switch spaces, etc.
	if navigationKeys[keyName] {
		flags |= flagNumPad
	}

	// When ctrl is involved, send proper kCGEventFlagsChanged events around the
	// key press. The Dock's space-switcher and other system handlers only respond
	// to kCGEventFlagsChanged for modifier tracking — a plain kCGEventKeyDown
	// with ctrl flags embedded is not enough for ctrl+arrow space switching.
	if hasCtrl {
		C.postModifier(C.int(kVK_Control), C.int(1), C.uint64_t(flags))
		C.postKey(C.int(keyCode), C.int(1), C.uint64_t(flags))
		C.postKey(C.int(keyCode), C.int(0), C.uint64_t(flags))
		C.postModifier(C.int(kVK_Control), C.int(0), 0)
		return nil
	}

	// Standard key press: down then up with modifier flags.
	C.postKey(C.int(keyCode), C.int(1), C.uint64_t(flags))
	C.postKey(C.int(keyCode), C.int(0), C.uint64_t(flags))
	return nil
}

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CGEvent.h>
#include <CoreGraphics/CGEventTypes.h>
#include <CoreGraphics/CGEventSource.h>
#include <CoreFoundation/CoreFoundation.h>

// postKey posts a regular key down or key up event with the given flags.
// Used for non-modifier keys (letters, arrows, function keys, etc.).
void postKey(int keyCode, int down, uint64_t flags) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
    CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)keyCode, down);
    CGEventSetFlags(e, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    if (src) CFRelease(src);
}

// postModifier sends a kCGEventFlagsChanged event for a modifier key.
//
// Real keyboards NEVER send kCGEventKeyDown/Up for modifier keys — they always
// send kCGEventFlagsChanged. The Dock (which handles ctrl+arrow for space
// switching) and WindowServer both specifically watch for kCGEventFlagsChanged
// to track the modifier state. Sending a regular kCGEventKeyDown for ctrl is
// ignored by the Dock, which is why ctrl+left never switched spaces.
//
// flags should include the modifier's own flag when pressing (down=1) and
// omit it when releasing (down=0), plus any other already-held modifiers.
void postModifier(int keyCode, int down, uint64_t flags) {
    CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateCombinedSessionState);
    CGEventRef e = CGEventCreate(src);
    CGEventSetType(e, kCGEventFlagsChanged);
    CGEventSetIntegerValueField(e, kCGKeyboardEventKeycode, (int64_t)keyCode);
    CGEventSetFlags(e, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    if (src) CFRelease(src);
}
*/
