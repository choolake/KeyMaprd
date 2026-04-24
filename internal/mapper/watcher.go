package mapper

import (
"fmt"
"path/filepath"
"sync"

"github.com/fsnotify/fsnotify"
)

// Watcher holds the current Config and watches the config file for changes.
// It is safe to call Lookup concurrently from multiple goroutines.
type Watcher struct {
path    string
mu      sync.RWMutex // protects cfg
cfg     *Config
watcher *fsnotify.Watcher
}

// NewWatcher loads the config at path and starts watching it for changes.
// When the file is modified, the config is automatically reloaded.
// Call Close() to stop watching.
func NewWatcher(path string) (*Watcher, error) {
cfg, err := LoadConfig(path)
if err != nil {
return nil, err
}

fw, err := fsnotify.NewWatcher()
if err != nil {
return nil, fmt.Errorf("create fsnotify watcher: %w", err)
}

// Watch the directory instead of the file directly.
// Editors often write configs by replacing the file (write to temp → rename),
// which would remove the watch on the original inode. Watching the directory
// catches all writes including renames.
dir := filepath.Dir(path)
if err := fw.Add(dir); err != nil {
fw.Close()
return nil, fmt.Errorf("watch dir %q: %w", dir, err)
}

w := &Watcher{path: path, cfg: cfg, watcher: fw}
go w.watch(path)
return w, nil
}

// Lookup returns the configured action for the given button number,
// or an empty string if no mapping exists.
// Safe to call concurrently.
func (w *Watcher) Lookup(button uint8) string {
w.mu.RLock()
defer w.mu.RUnlock()
return w.cfg.Buttons[fmt.Sprintf("btn%d", button)]
}

// MappingCount returns the number of configured button mappings.
func (w *Watcher) MappingCount() int {
w.mu.RLock()
defer w.mu.RUnlock()
return len(w.cfg.Buttons)
}

// Close stops the file watcher.
func (w *Watcher) Close() {
w.watcher.Close()
}

// watch runs in a goroutine, listening for file system events and
// reloading the config when the watched file changes.
func (w *Watcher) watch(path string) {
for {
select {
case event, ok := <-w.watcher.Events:
if !ok {
return // watcher closed
}
// Only react to the specific config file, not other files in the dir.
if event.Name != path {
continue
}
if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
w.reload()
}
case err, ok := <-w.watcher.Errors:
if !ok {
return
}
fmt.Printf("config watcher error: %v\n", err)
}
}
}

// reload reads the config file and atomically swaps in the new config.
func (w *Watcher) reload() {
cfg, err := LoadConfig(w.path)
if err != nil {
fmt.Printf("config reload failed (keeping previous config): %v\n", err)
return
}
w.mu.Lock()
w.cfg = cfg
w.mu.Unlock()
fmt.Printf("config reloaded — %d mapping(s) active\n", len(cfg.Buttons))
}
