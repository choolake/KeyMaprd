//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const plistLabel = "com.choolake.keymaprd"

// plistTemplate is the LaunchAgent plist that launchd uses to auto-start keymaprd on login.
// It runs as the current user (user-level agent, not system daemon).
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{.Label}}</string>

  <key>ProgramArguments</key>
  <array>
    <string>{{.BinaryPath}}</string>
    <string>--config</string>
    <string>{{.ConfigPath}}</string>
  </array>

  <!-- Restart automatically if it crashes -->
  <key>KeepAlive</key>
  <true/>

  <!-- Start at login -->
  <key>RunAtLoad</key>
  <true/>

  <!-- Write stdout/stderr to log files for debugging -->
  <key>StandardOutPath</key>
  <string>{{.LogDir}}/keymaprd.log</string>
  <key>StandardErrorPath</key>
  <string>{{.LogDir}}/keymaprd.error.log</string>
</dict>
</plist>
`

type plistData struct {
	Label      string
	BinaryPath string
	ConfigPath string
	LogDir     string
}

// plistPath returns the canonical location for the LaunchAgent plist.
// LaunchAgents in ~/Library/LaunchAgents are loaded per-user at login.
func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

// logDir returns the directory where keymaprd writes its log files.
func logDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "keymaprd")
}

// defaultConfigPath returns ~/.config/keymaprd/config.json (reuse mapper default).
func defaultInstallConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "keymaprd", "config.json")
}

// runInstall writes the LaunchAgent plist and loads it with launchctl.
func runInstall() {
	// Resolve the absolute path to the currently running binary.
	binaryPath, err := os.Executable()
	if err != nil {
		fatalf("could not determine binary path: %v", err)
	}
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		fatalf("could not resolve binary symlinks: %v", err)
	}

	// Ensure log directory exists.
	logD := logDir()
	if err := os.MkdirAll(logD, 0755); err != nil {
		fatalf("could not create log directory %s: %v", logD, err)
	}

	// Write the plist.
	dest := plistPath()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		fatalf("could not create LaunchAgents directory: %v", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		fatalf("could not create plist %s: %v", dest, err)
	}
	defer f.Close()

	tmpl := template.Must(template.New("plist").Parse(plistTemplate))
	if err := tmpl.Execute(f, plistData{
		Label:      plistLabel,
		BinaryPath: binaryPath,
		ConfigPath: defaultInstallConfigPath(),
		LogDir:     logD,
	}); err != nil {
		fatalf("could not write plist: %v", err)
	}

	fmt.Printf("Wrote plist → %s\n", dest)

	// Load the agent (launchctl bootstrap registers it with the current session).
	if err := launchctlLoad(dest); err != nil {
		fatalf("launchctl load failed: %v\n\nYou can load it manually with:\n  launchctl load %s", err, dest)
	}

	fmt.Println("✓ keymaprd is installed and running as a LaunchAgent.")
	fmt.Println("  It will start automatically at every login.")
	fmt.Printf("  Logs: %s/keymaprd.log\n", logD)
}

// runUninstall stops the daemon and removes the plist.
func runUninstall() {
	dest := plistPath()

	// Unload first (ignore error — it might not be loaded).
	_ = launchctlUnload(dest)

	if err := os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("keymaprd is not installed (plist not found).")
			return
		}
		fatalf("could not remove plist %s: %v", dest, err)
	}

	fmt.Println("✓ keymaprd has been uninstalled.")
	fmt.Println("  The plist has been removed; it will not start at next login.")
}

// userDomain returns the launchctl user domain target, e.g. "gui/501".
func userDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// launchctlLoad bootstraps (enables + starts) a LaunchAgent plist.
// Uses the modern `launchctl bootstrap` API (replaces deprecated `load`).
// If the service is already loaded, it boots it out first so the new plist
// takes effect cleanly.
func launchctlLoad(plistFile string) error {
	domain := userDomain()
	// Silently remove any existing registration so a fresh bootstrap works.
	_ = exec.Command("launchctl", "bootout", domain+"/"+plistLabel).Run()

	out, err := exec.Command("launchctl", "bootstrap", domain, plistFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(out))
	}
	return nil
}

// launchctlUnload boots out (stops + disables) the LaunchAgent.
// Uses the modern `launchctl bootout` API (replaces deprecated `unload`).
func launchctlUnload(_ string) error {
	out, err := exec.Command("launchctl", "bootout", userDomain()+"/"+plistLabel).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(out))
	}
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
