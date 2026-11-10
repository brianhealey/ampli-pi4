package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// streamsScriptsDirV is set by the Manager so stream implementations can find
// bundled scripts and binaries. It is read from supervisor goroutines, hence
// atomic.
var streamsScriptsDirV atomic.Value // string

func setStreamsScriptsDir(dir string) { streamsScriptsDirV.Store(dir) }

func streamsScriptsDir() string {
	dir, _ := streamsScriptsDirV.Load().(string)
	return dir
}

// findBinary searches for a binary by name in order:
//  1. exec.LookPath (PATH)
//  2. /usr/bin/<name>
//  3. streamsScriptsDir/<name>
func findBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if p := filepath.Join("/usr/bin", name); fileExists(p) {
		return p
	}
	if dir := streamsScriptsDir(); dir != "" {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p
		}
	}
	// Return the name and let exec.Command fail naturally with a clear error
	return name
}

// fileExists returns true if the path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// buildConfigDir creates ~/.config/amplipi/srcs/v{vsrc}/ and returns the path.
func buildConfigDir(base string, vsrc int) (string, error) {
	dir := filepath.Join(base, fmt.Sprintf("v%d", vsrc))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("buildConfigDir %s: %w", dir, err)
	}
	return dir, nil
}

// writeFileAtomic writes content to a file atomically (write temp, rename).
func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// bridge is the running connection from a stream's loopback to its source's
// DAC: an ALSALoop, or an eqBridge (CamillaDSP) when the source has EQ.
type bridge interface {
	Start(ctx context.Context) error
	Stop() error
}

// bridgeFactory builds the bridge for vsrc -> physSrc. The Manager provides
// one that picks CamillaDSP for sources with an enabled EQ; without it every
// connection uses alsaloop.
type bridgeFactory func(vsrc, physSrc int) (bridge, error)

// bridgeSetter is implemented by streams that bridge through the loopback.
type bridgeSetter interface {
	setBridgeFactory(f bridgeFactory)
}

// newALSALoopBridge is the default bridge: alsaloop, exactly as before EQ.
func newALSALoopBridge(vsrc, physSrc int) (bridge, error) {
	l, err := NewALSALoop(vsrc, physSrc)
	if err != nil {
		return nil, err // never a typed-nil interface
	}
	return l, nil
}

// SubprocStream embeds a Supervisor and ALSALoop.
// Stream types can embed this and override what they need.
type SubprocStream struct {
	sup       *Supervisor
	loop      bridge
	newBridge bridgeFactory // nil: alsaloop
	vsrc      int
	configDir string

	mu   sync.RWMutex
	info models.StreamInfo
}

// activateBase starts the ALSA loop for a connected stream and
// sets up a supervisor. Called by stream-specific Activate methods
// after they have set sup.
func (ss *SubprocStream) activateBase(ctx context.Context, vsrc int, configDir string) error {
	ss.vsrc = vsrc
	ss.configDir = configDir
	if ss.sup != nil {
		if err := ss.sup.Start(ctx); err != nil {
			return fmt.Errorf("supervisor start: %w", err)
		}
	}
	return nil
}

// deactivateBase stops the subprocess and the loop.
func (ss *SubprocStream) deactivateBase(ctx context.Context) error {
	if ss.loop != nil {
		if err := ss.loop.Stop(); err != nil {
			slog.Warn("deactivateBase: loop stop error", "err", err)
		}
		ss.loop = nil
	}
	if ss.sup != nil {
		if err := ss.sup.Stop(); err != nil {
			slog.Warn("deactivateBase: supervisor stop error", "err", err)
		}
		ss.sup = nil
	}
	return nil
}

// connectBase starts the ALSA loop.
func (ss *SubprocStream) connectBase(ctx context.Context, physSrc int) error {
	if ss.loop != nil {
		_ = ss.loop.Stop()
	}
	newBridge := ss.newBridge
	if newBridge == nil {
		newBridge = newALSALoopBridge
	}
	loop, err := newBridge(ss.vsrc, physSrc)
	if err != nil {
		return fmt.Errorf("alsaloop creation failed: %w", err)
	}
	ss.loop = loop
	return ss.loop.Start(ctx)
}

func (ss *SubprocStream) setBridgeFactory(f bridgeFactory) { ss.newBridge = f }

// disconnectBase stops the ALSA loop.
func (ss *SubprocStream) disconnectBase(ctx context.Context) error {
	if ss.loop != nil {
		err := ss.loop.Stop()
		ss.loop = nil
		return err
	}
	return nil
}

// setInfo updates the stream info thread-safely.
func (ss *SubprocStream) setInfo(info models.StreamInfo) {
	ss.mu.Lock()
	ss.info = info
	ss.mu.Unlock()
}

// getInfo returns the current stream info thread-safely.
func (ss *SubprocStream) getInfo() models.StreamInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.info
}
