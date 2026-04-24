module github.com/choolake/keymaprd

go 1.26.2

// go-hid is used only for HID device enumeration (--list-devices).
// TODO Sprint 2: replace with native CGo IOKit enumeration to remove this dependency.
require github.com/sstallion/go-hid v0.15.0

require github.com/fsnotify/fsnotify v1.9.0

require golang.org/x/sys v0.13.0 // indirect
