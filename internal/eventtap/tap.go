//go:build darwin

// Package eventtap uses macOS CGEventTap to observe mouse button events
// at the session level. This is the correct macOS approach — it works
// with Bluetooth mice and doesn't require opening raw HID devices.
//
// Requires two macOS privacy permissions for the binary:
//   1. System Settings → Privacy & Security → Accessibility
//   2. System Settings → Privacy & Security → Input Monitoring
//
// Both are needed: Accessibility allows CGEventTapCreate to succeed;
// Input Monitoring (kTCCServiceListenEvent) is checked by WindowServer before
// delivering events to the tap. Without it, the tap creates fine but callbacks
// never fire — especially when running as a LaunchAgent service.
package eventtap

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation

#include <CoreGraphics/CGEvent.h>
#include <CoreGraphics/CGEventTypes.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdint.h>
#include <unistd.h>

// tapPipe[0] = read end (Go reads from here)
// tapPipe[1] = write end (C callback writes here)
static int tapPipe[2] = {-1, -1};
static CFMachPortRef eventTap = NULL;
static CFRunLoopSourceRef runLoopSource = NULL;

// Each mouse event is encoded as 2 bytes written to the pipe:
//   byte 0: 0 = button down, 1 = button up
//   byte 1: button number (0=left, 1=right, 2=middle, 3+=extra)
CGEventRef handleEvent(CGEventTapProxy proxy, CGEventType type,
                       CGEventRef event, void *refcon) {
    if (type == kCGEventTapDisabledByTimeout) {
        CGEventTapEnable(eventTap, true);
        return event;
    }

    int64_t btn = CGEventGetIntegerValueField(event, kCGMouseEventButtonNumber);
    uint8_t msg[2];

    if (type == kCGEventLeftMouseDown  ||
        type == kCGEventRightMouseDown ||
        type == kCGEventOtherMouseDown) {
        msg[0] = 0; // down
    } else {
        msg[0] = 1; // up
    }
    msg[1] = (uint8_t)(btn & 0xFF);

    write(tapPipe[1], msg, 2);
    return event;
}

// startTap creates the pipe and the event tap.
// Returns the read-end fd on success, -1 on pipe error, -2 if
// CGEventTapCreate fails (usually missing Accessibility permission).
int startTap() {
    if (pipe(tapPipe) != 0) return -1;

    CGEventMask mask =
        CGEventMaskBit(kCGEventLeftMouseDown)  | CGEventMaskBit(kCGEventLeftMouseUp)  |
        CGEventMaskBit(kCGEventRightMouseDown) | CGEventMaskBit(kCGEventRightMouseUp) |
        CGEventMaskBit(kCGEventOtherMouseDown) | CGEventMaskBit(kCGEventOtherMouseUp);

    eventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        handleEvent,
        NULL
    );
    if (!eventTap) return -2;

    runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, eventTap, 0);
    return tapPipe[0];
}

// runLoop attaches the run loop source to the CURRENT thread's run loop and
// then blocks running it. This must be called on the same thread that will
// service the tap — never call CFRunLoopGetMain() here, since Go goroutines
// don't run on the process main thread.
void runLoop() {
    CFRunLoopAddSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(eventTap, true);
    CFRunLoopRun();
}

// stopTap disables the tap and stops the run loop.
void stopTap() {
    if (eventTap) {
        CGEventTapEnable(eventTap, false);
        CFMachPortInvalidate(eventTap);
        CFRelease(eventTap);
        eventTap = NULL;
    }
    if (runLoopSource) {
        CFRunLoopRemoveSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopCommonModes);
        CFRelease(runLoopSource);
        runLoopSource = NULL;
    }
    if (tapPipe[1] != -1) { close(tapPipe[1]); tapPipe[1] = -1; }
    if (tapPipe[0] != -1) { close(tapPipe[0]); tapPipe[0] = -1; }
    CFRunLoopStop(CFRunLoopGetCurrent());
}
*/
import "C"

import (
"fmt"
"io"
"os"
"runtime"
)

// Event represents a single mouse button press or release.
type Event struct {
Button uint8 // 0=left, 1=right, 2=middle, 3+=extra buttons
Down   bool  // true = pressed, false = released
}

// Start registers a CGEventTap and returns a channel that receives mouse
// button events. A background goroutine locked to its OS thread runs the
// CoreFoundation run loop required by the tap.
//
// Call Stop() to clean up when done.
func Start() (<-chan Event, error) {
readFd := C.startTap()
switch readFd {
case -1:
return nil, fmt.Errorf("pipe creation failed")
case -2:
return nil, fmt.Errorf("CGEventTapCreate failed — grant both permissions in System Settings → Privacy & Security:\n  1. Accessibility     → add keymaprd\n  2. Input Monitoring  → add keymaprd")
}

events := make(chan Event, 64)
f := os.NewFile(uintptr(readFd), "eventtap-pipe")

// The CoreFoundation run loop must run on the same OS thread where we attach
// the run loop source. runtime.LockOSThread() ensures this goroutine owns
// its OS thread permanently — no other goroutine will be scheduled on it.
go func() {
runtime.LockOSThread()
C.runLoop() // blocks until stopTap() calls CFRunLoopStop
}()

// Read 2-byte messages from the pipe and forward them as Event values.
go func() {
defer close(events)
buf := make([]byte, 2)
for {
if _, err := io.ReadFull(f, buf); err != nil {
return
}
events <- Event{
Down:   buf[0] == 0,
Button: buf[1],
}
}
}()

return events, nil
}

// Stop tears down the event tap and closes the event channel.
func Stop() {
C.stopTap()
}
