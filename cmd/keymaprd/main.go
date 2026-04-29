//go:build darwin

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/choolake/KeyMaprd/internal/eventtap"
	"github.com/choolake/KeyMaprd/internal/hid"
	"github.com/choolake/KeyMaprd/internal/inject"
	"github.com/choolake/KeyMaprd/internal/mapper"
	"github.com/choolake/KeyMaprd/internal/setup"
)

const version = "0.2.0"

func main() {
	// Handle subcommands before flag parsing — subcommands have no flags of their own.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "install":
			runInstall()
			return
		case "uninstall":
			runUninstall()
			return
		case "setup":
			runSetup()
			return
		}
	}

	// --- CLI flags ---
	var (
		flagVersion     = flag.Bool("version", false, "Print version and exit")
		flagListDevices = flag.Bool("list-devices", false, "List all connected HID devices and exit")
		flagDump        = flag.Bool("dump", false, "Print all mouse button events (use this to discover button numbers for config.json)")
		flagConfig      = flag.String("config", "", "Path to config.json (default: ~/.config/keymaprd/config.json)")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("keymaprd v%s\n", version)
		os.Exit(0)
	}

	if *flagListDevices {
		runListDevices()
		os.Exit(0)
	}

	if *flagDump {
		runDump()
		os.Exit(0)
	}

	runStart(resolveConfigPath(*flagConfig))
}

// resolveConfigPath returns the explicit path if non-empty, otherwise the default.
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return mapper.DefaultConfigPath()
}

// runListDevices prints a table of every connected HID device.
func runListDevices() {
	devices, err := hid.ListDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(devices) == 0 {
		fmt.Println("No HID devices found.")
		return
	}
	fmt.Printf("%-6s %-6s %-10s %-6s  %-30s  %s\n",
		"VID", "PID", "UsagePage", "Usage", "Product", "Path")
	fmt.Println("-----------------------------------------------------------------------")
	for _, d := range devices {
		fmt.Printf("0x%04X 0x%04X 0x%04X     0x%04X  %-30s  %s\n",
			d.VendorID, d.ProductID, d.UsagePage, d.Usage, d.Product, d.Path)
	}
}

// runDump starts an event tap and prints every mouse button event as it arrives.
// Press each button on your mouse and note the button number shown — use these
// as keys in config.json (e.g. "btn3": "cmd+space").
func runDump() {
	fmt.Println("Starting event tap… (press Ctrl+C to stop)")
	fmt.Println("Press each button on your mouse and note the button number below.")
	fmt.Println("Tip: left=0, right=1, middle=2, extra buttons are 3 and above.")
	fmt.Println()

	events, err := eventtap.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer eventtap.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			state := "UP  "
			if e.Down {
				state = "DOWN"
			}
			fmt.Printf("Button %-3d %s  → config key: \"btn%d\"\n", e.Button, state, e.Button)
		case <-sig:
			fmt.Println("\nStopped.")
			return
		}
	}
}

// runStart loads config, starts watching it for changes, and runs the main remapping loop.
// On first run (no config.json), it launches the interactive TUI setup wizard.
func runStart(configPath string) {
	fmt.Printf("keymaprd v%s starting…\n", version)
	fmt.Printf("Config: %s\n\n", configPath)

	// If config does not exist, launch the interactive setup wizard.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("No config found — launching setup wizard\u2026")
		result := setup.Run()
		if !result.Saved {
			// User cancelled the wizard — nothing to do.
			fmt.Println("Setup cancelled.")
			os.Exit(0)
		}
		if result.InstallDaemon {
			// User chose to install as a LaunchAgent — do it now before starting.
			// launchd will re-launch us automatically; exit to avoid double-running.
			runInstall()
			os.Exit(0)
		}
		// Wizard wrote the config — fall through and start normally.
		fmt.Println()
	}

	// NewWatcher loads the config and starts hot-reload in the background.
	w, err := mapper.NewWatcher(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n\n", err)
		fmt.Fprintf(os.Stderr, "To get started, copy the example config:\n")
		fmt.Fprintf(os.Stderr, "  mkdir -p ~/.config/keymaprd\n")
		fmt.Fprintf(os.Stderr, "  cp /opt/homebrew/share/keymaprd/config.example.json ~/.config/keymaprd/config.json\n\n")
		fmt.Fprintf(os.Stderr, "Or if installed from source:\n")
		fmt.Fprintf(os.Stderr, "  cp config.example.json ~/.config/keymaprd/config.json\n\n")
		fmt.Fprintf(os.Stderr, "Then run 'keymaprd --dump' to discover your button numbers.\n")
		os.Exit(1)
	}
	defer w.Close()

	fmt.Printf("Loaded %d button mapping(s). Edit %s to remap — changes apply instantly.\n\n", w.MappingCount(), configPath)

	events, err := eventtap.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer eventtap.Stop()

	fmt.Println("Listening for mouse buttons. Press Ctrl+C to stop.\n")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			if !e.Down {
				continue // only act on button press, not release
			}
			action := w.Lookup(e.Button)
			if action == "" {
				continue // unmapped button — ignore
			}
			fmt.Printf("btn%d → %q\n", e.Button, action)
			if err := inject.Press(action); err != nil {
				fmt.Fprintf(os.Stderr, "inject error: %v\n", err)
			}
		case <-sig:
			fmt.Println("Shutting down.")
			return
		}
	}
}

// runSetup launches the interactive TUI config wizard explicitly.
// Users can re-run it at any time with: keymaprd setup
func runSetup() {
	result := setup.Run()
	if !result.Saved {
		fmt.Println("Setup cancelled — no changes made.")
		return
	}
	fmt.Println("Config saved.")
	if result.InstallDaemon {
		runInstall()
	}
}
