//go:build darwin

// Package hid wraps go-hid (libusb/hidapi) for device enumeration only.
// Actual button events are captured via CGEventTap (see internal/eventtap).
package hid

import (
"fmt"

gohid "github.com/sstallion/go-hid"
)

// DeviceInfo holds human-readable info about a connected HID device.
type DeviceInfo struct {
VendorID     uint16
ProductID    uint16
UsagePage    uint16
Usage        uint16
Path         string
Manufacturer string
Product      string
}

// ListDevices enumerates every HID device currently connected to the system.
// This powers the --list-devices CLI flag.
func ListDevices() ([]DeviceInfo, error) {
if err := gohid.Init(); err != nil {
return nil, fmt.Errorf("hid init: %w", err)
}
defer gohid.Exit()

var devices []DeviceInfo
err := gohid.Enumerate(gohid.VendorIDAny, gohid.ProductIDAny, func(info *gohid.DeviceInfo) error {
devices = append(devices, DeviceInfo{
VendorID:     info.VendorID,
ProductID:    info.ProductID,
UsagePage:    info.UsagePage,
Usage:        info.Usage,
Path:         info.Path,
Manufacturer: info.MfrStr,
Product:      info.ProductStr,
})
return nil
})
if err != nil {
return nil, fmt.Errorf("enumerate: %w", err)
}
return devices, nil
}
