package streams

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// procAsoundRoot is where the kernel exposes ALSA PCM state. A variable so
// tests can point it at a fake tree.
var procAsoundRoot = "/proc/asound"

// Audio-output watchdog timing for players that should hold their PCM open
// for as long as they run. The grace period covers connecting and buffering a
// network stream; two consecutive closed readings ride out a brief reopen
// (e.g. a format change between playlist items).
const (
	pcmWatchGrace    = 20 * time.Second
	pcmWatchInterval = 5 * time.Second
	pcmWatchFailures = 2
)

// errPCMUnknown means the PCM state cannot be read (no /proc/asound on macOS,
// in mock mode or tests, or the loopback card is missing). The watchdog then
// does nothing.
var errPCMUnknown = errors.New("pcm state unknown")

// playbackPCMOpen reports whether any playback substream of card/device is
// open, from /proc/asound/<card>/pcm<device>p/sub*/status ("closed" when not
// open).
func playbackPCMOpen(root, card string, device int) (bool, error) {
	pattern := filepath.Join(root, card, fmt.Sprintf("pcm%dp", device), "sub*", "status")
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return false, errPCMUnknown
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return false, errPCMUnknown
		}
		if strings.TrimSpace(string(data)) != "closed" {
			return true, nil
		}
	}
	return false, nil
}

// audioOutputCheck returns a Supervisor health check that fails when the
// player on vsrc does not have its loopback playback PCM open. It never fails
// when the state cannot be read.
func audioOutputCheck(vsrc int) func() error {
	card, device := VirtualOutputPCM(vsrc)
	return func() error {
		open, err := playbackPCMOpen(procAsoundRoot, card, device)
		if err != nil || open {
			return nil
		}
		return fmt.Errorf("audio output %s (hw:%s,%d playback) is not open",
			VirtualOutputDevice(vsrc), card, device)
	}
}

// watchAudioOutput enables the audio-output watchdog on sup. Only for players
// that keep their PCM open continuously while running (internet radio);
// receivers such as AirPlay, Spotify, LMS, Pandora, DLNA and Bluetooth close
// it while idle, so they must not use it.
func watchAudioOutput(sup *Supervisor, vsrc int) {
	sup.healthCheck = audioOutputCheck(vsrc)
	sup.healthGrace = pcmWatchGrace
	sup.healthInterval = pcmWatchInterval
	sup.healthFailures = pcmWatchFailures
}
