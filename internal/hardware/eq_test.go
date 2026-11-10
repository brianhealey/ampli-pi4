package hardware

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectEQBinary(t *testing.T) {
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	dir := t.TempDir()
	exe := filepath.Join(dir, "camilladsp")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := detectEQBinary(func(string) (string, error) { return "/opt/bin/camilladsp", nil }, exe); got != "/opt/bin/camilladsp" {
		t.Errorf("PATH hit: got %q", got)
	}
	if got := detectEQBinary(notFound, exe); got != exe {
		t.Errorf("install path fallback: got %q", got)
	}
	if got := detectEQBinary(notFound, plain); got != "" {
		t.Errorf("non-executable file accepted: %q", got)
	}
	if got := detectEQBinary(notFound, filepath.Join(dir, "missing")); got != "" {
		t.Errorf("missing binary reported as %q", got)
	}

	var nilProfile *HardwareProfile
	if nilProfile.EQAvailable() || (&HardwareProfile{}).EQAvailable() || !MockProfile().EQAvailable() {
		t.Error("EQAvailable wrong for nil / empty / mock profile")
	}
}
