//go:build darwin

// Package hid provides HID device enumeration using the native macOS IOKit
// framework via CGo. No external libraries (hidapi, go-hid) are required.
// Actual button events are captured via CGEventTap (see internal/eventtap).
package hid

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation

#include <IOKit/hid/IOHIDManager.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

// cfStrToC copies a CFStringRef into a newly malloc'd C string.
// Returns NULL if cfStr is NULL. Caller must free() the result.
static char *cfStrToC(CFStringRef cfStr) {
    if (!cfStr) return NULL;
    CFIndex len = CFStringGetMaximumSizeForEncoding(
        CFStringGetLength(cfStr), kCFStringEncodingUTF8) + 1;
    char *buf = malloc(len);
    if (!buf) return NULL;
    if (!CFStringGetCString(cfStr, buf, len, kCFStringEncodingUTF8)) {
        free(buf);
        return NULL;
    }
    return buf;
}

// getIntPropStr reads an integer property using a plain C-string key.
// Returns 0 if absent or not a number.
static int32_t getIntPropStr(IOHIDDeviceRef dev, const char *key) {
    CFStringRef cfKey = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
    if (!cfKey) return 0;
    CFTypeRef val = IOHIDDeviceGetProperty(dev, cfKey);
    CFRelease(cfKey);
    if (!val || CFGetTypeID(val) != CFNumberGetTypeID()) return 0;
    int32_t n = 0;
    CFNumberGetValue((CFNumberRef)val, kCFNumberSInt32Type, &n);
    return n;
}

// getStrPropStr reads a string property using a plain C-string key.
// Returns NULL if absent. Caller must free() the result.
static char *getStrPropStr(IOHIDDeviceRef dev, const char *key) {
    CFStringRef cfKey = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
    if (!cfKey) return NULL;
    CFTypeRef val = IOHIDDeviceGetProperty(dev, cfKey);
    CFRelease(cfKey);
    if (!val || CFGetTypeID(val) != CFStringGetTypeID()) return NULL;
    return cfStrToC((CFStringRef)val);
}

// setMatchAll tells the IOHIDManager to match every HID device.
// Passing NULL directly from CGo is tricky; this wrapper makes it explicit.
static void setMatchAll(IOHIDManagerRef mgr) {
    IOHIDManagerSetDeviceMatching(mgr, NULL);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// IOKit HID property key strings (values of the kIOHID*Key #defines).
// We pass these as plain C strings to avoid CGo issues with CFStringRef constants.
const (
	kVendorID     = "VendorID"
	kProductID    = "ProductID"
	kUsagePage    = "PrimaryUsagePage"
	kUsage        = "PrimaryUsage"
	kManufacturer = "Manufacturer"
	kProduct      = "Product"
	kTransport    = "Transport"
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

// ListDevices enumerates every HID device currently connected to the system
// using the native macOS IOKit HID Manager. No external dependencies required.
// This powers the --list-devices CLI flag.
func ListDevices() ([]DeviceInfo, error) {
	// IOHIDManagerCreate allocates a manager that can query all HID devices.
	// kIOHIDOptionsTypeNone means we only want to observe, not exclusively own.
	mgr := C.IOHIDManagerCreate(C.kCFAllocatorDefault, C.kIOHIDOptionsTypeNone)
	if mgr == 0 {
		return nil, fmt.Errorf("IOHIDManagerCreate returned nil")
	}
	defer C.CFRelease(C.CFTypeRef(unsafe.Pointer(mgr)))

	// nil matching dict = match ALL HID devices (no vendor/product filter).
	C.setMatchAll(mgr)

	// IOHIDManagerCopyDevices returns a CFSetRef of all currently connected devices.
	devSet := C.IOHIDManagerCopyDevices(mgr)
	if devSet == 0 {
		return nil, fmt.Errorf("IOHIDManagerCopyDevices returned nil (no HID devices found)")
	}
	defer C.CFRelease(C.CFTypeRef(unsafe.Pointer(devSet)))

	count := int(C.CFSetGetCount(devSet))
	if count == 0 {
		return nil, nil
	}

	// Copy the CFSet values into a Go slice via a temporary C array.
	vals := make([]unsafe.Pointer, count)
	C.CFSetGetValues(devSet, (*unsafe.Pointer)(unsafe.Pointer(&vals[0])))

	devices := make([]DeviceInfo, 0, count)
	for _, ptr := range vals {
		dev := C.IOHIDDeviceRef(ptr)

		vid := C.getIntPropStr(dev, C.CString(kVendorID))
		pid := C.getIntPropStr(dev, C.CString(kProductID))
		up := C.getIntPropStr(dev, C.CString(kUsagePage))
		u := C.getIntPropStr(dev, C.CString(kUsage))

		mfrC := C.getStrPropStr(dev, C.CString(kManufacturer))
		prodC := C.getStrPropStr(dev, C.CString(kProduct))
		pathC := C.getStrPropStr(dev, C.CString(kTransport))

		mfr, prod, path := "", "", ""
		if mfrC != nil {
			mfr = C.GoString(mfrC)
			C.free(unsafe.Pointer(mfrC))
		}
		if prodC != nil {
			prod = C.GoString(prodC)
			C.free(unsafe.Pointer(prodC))
		}
		if pathC != nil {
			path = C.GoString(pathC)
			C.free(unsafe.Pointer(pathC))
		}

		devices = append(devices, DeviceInfo{
			VendorID:     uint16(vid),
			ProductID:    uint16(pid),
			UsagePage:    uint16(up),
			Usage:        uint16(u),
			Path:         path,
			Manufacturer: mfr,
			Product:      prod,
		})
	}
	return devices, nil
}
