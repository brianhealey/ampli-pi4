package identity_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/identity"
)

func TestGetVersion_Fallback(t *testing.T) {
	// Use a temp dir that contains no metadata.json
	dir := t.TempDir()
	got := identity.GetVersionFromDir(dir)
	if got != identity.DefaultVersion {
		t.Errorf("GetVersionFromDir(%q) = %q; want %q", dir, got, identity.DefaultVersion)
	}
}

func TestGetVersion_FromFile(t *testing.T) {
	dir := t.TempDir()
	want := "0.4.10"
	meta := map[string]interface{}{"version": want}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	got := identity.GetVersionFromDir(dir)
	if got != want {
		t.Errorf("GetVersionFromDir(%q) = %q; want %q", dir, got, want)
	}
}

func TestGetVersion_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	got := identity.GetVersionFromDir(dir)
	if got != identity.DefaultVersion {
		t.Errorf("GetVersionFromDir with invalid JSON = %q; want %q", got, identity.DefaultVersion)
	}
}

func TestGetOnlineStatus_Missing(t *testing.T) {
	// Neither status file exists; function should return false.
	// Note: this test is best-effort; if /tmp/amplipi-online or the Pi config
	// file happens to exist on the test machine, the result may differ.
	// We skip checking the actual value if either file exists.
	_, errGo := os.Stat("/tmp/amplipi-online")
	_, errPy := os.Stat("/home/pi/amplipi-dev/config/online")
	if errGo == nil || errPy == nil {
		t.Skip("status file exists on this machine; skipping offline test")
	}

	got := identity.GetOnlineStatus()
	if got {
		t.Error("GetOnlineStatus() = true; want false when no status files exist")
	}
}

func TestGetHostname(t *testing.T) {
	// Should not panic and should return a non-empty string
	h := identity.GetHostname()
	if h == "" {
		t.Error("GetHostname() returned empty string")
	}
}

func TestIsUpdateMode_NoFlag(t *testing.T) {
	// Ensure the flag file doesn't exist
	os.Remove("/tmp/amplipi-update.flag")
	// Can't easily test the exe-path check without renaming ourselves,
	// but we can verify the function doesn't panic.
	_ = identity.IsUpdateMode()
}

func TestIsUpdateMode_WithFlag(t *testing.T) {
	f := "/tmp/amplipi-update.flag"
	if err := os.WriteFile(f, []byte{}, 0644); err != nil {
		t.Skip("cannot write to /tmp: " + err.Error())
	}
	t.Cleanup(func() { os.Remove(f) })

	if !identity.IsUpdateMode() {
		t.Error("IsUpdateMode() = false; want true when flag file exists")
	}
}
