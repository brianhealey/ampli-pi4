package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/micro-nova/amplipi-go/internal/maintenance"
	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.ctrl.GetInfo())
}

func (h *Handlers) factoryReset(w http.ResponseWriter, r *http.Request) {
	state, appErr := h.ctrl.FactoryReset(r.Context())
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) loadConfig(w http.ResponseWriter, r *http.Request) {
	var incoming models.State
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.LoadConfig(r.Context(), incoming)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// loginPage renders a simple login HTML page.
func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/api"
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>AmpliPi Login</title></head>
<body>
<h2>AmpliPi Login</h2>
<form method="POST" action="/auth/login">
  <input type="hidden" name="next" value="` + next + `">
  <label>Password: <input type="password" name="password"></label>
  <button type="submit">Login</button>
</form>
</body>
</html>`))
}

// loginPost handles login form submission.
// TODO: implement proper credential verification with argon2.
func (h *Handlers) loginPost(w http.ResponseWriter, r *http.Request) {
	// For now, redirect to requested URL (auth service handles actual verification).
	next := r.FormValue("next")
	if next == "" {
		next = "/api"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// testPreamp runs a quick preamp self-test (read version reg from all units).
func (h *Handlers) testPreamp(w http.ResponseWriter, r *http.Request) {
	result, err := h.ctrl.TestPreamp(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	status := http.StatusOK
	if ok, _ := result["ok"].(bool); !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, result)
}

// testFans forces fans on for 3 seconds via REG_FANS, then returns to auto.
func (h *Handlers) testFans(w http.ResponseWriter, r *http.Request) {
	result, err := h.ctrl.TestFans(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	status := http.StatusOK
	if ok, _ := result["ok"].(bool); !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, result)
}

// flashFirmware is a stub — firmware flashing is not yet implemented in the Go version.
func (h *Handlers) flashFirmware(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]interface{}{
		"error": "firmware flashing not yet implemented in Go version; use the updater service",
	})
}

// createBackup triggers an immediate config backup and returns the file path.
func (h *Handlers) createBackup(w http.ResponseWriter, r *http.Request) {
	svc := maintenance.New("", nil, nil)
	file, err := svc.RunBackupNow()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file": file,
	})
}

// listBackups returns a list of available backup files.
func (h *Handlers) listBackups(w http.ResponseWriter, r *http.Request) {
	files, err := maintenance.ListBackups()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"backups": files,
	})
}

// restoreBackup accepts a multipart file upload, extracts it to ~/.config/amplipi/.
func (h *Handlers) restoreBackup(w http.ResponseWriter, r *http.Request) {
	// Limit upload size to 100 MB
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "failed to parse multipart form: " + err.Error(),
		})
		return
	}

	file, header, err := r.FormFile("backup")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "missing backup file in form field 'backup': " + err.Error(),
		})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(header.Filename, ".tar.gz") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "backup file must be a .tar.gz archive",
		})
		return
	}

	// Save the upload to a temp file for extraction
	tmp, err := os.CreateTemp("", "amplipi-restore-*.tar.gz")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to create temp file: " + err.Error(),
		})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "failed to save uploaded file: " + err.Error(),
		})
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "seek failed: " + err.Error(),
		})
		return
	}

	// Determine destination directory
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "cannot determine home directory: " + err.Error(),
		})
		return
	}
	destDir := filepath.Join(home, ".config", "amplipi")

	if err := extractTarGz(tmp, destDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "extraction failed: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"details": fmt.Sprintf("restored to %s at %s", destDir, time.Now().Format(time.RFC3339)),
	})
}

// extractTarGz extracts a .tar.gz archive from r into destDir.
// Only extracts regular files, with path sanitization to prevent path traversal.
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// Sanitize the path: strip leading slashes and any ".." components
		cleanName := filepath.Clean(filepath.Base(hdr.Name))
		if strings.Contains(cleanName, "..") {
			continue // skip suspicious entries
		}

		dest := filepath.Join(destDir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			f, err := os.Create(dest)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
