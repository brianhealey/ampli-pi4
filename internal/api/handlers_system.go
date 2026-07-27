package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/micro-nova/amplipi-go/internal/auth"
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
	// Sanitize the next parameter to prevent XSS
	safeNext := html.EscapeString(next)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>AmpliPi Login</title></head>
<body>
<h2>AmpliPi Login</h2>
<form method="POST" action="/auth/login">
  <input type="hidden" name="next" value="` + safeNext + `">
  <label>Password: <input type="password" name="password"></label>
  <button type="submit">Login</button>
</form>
</body>
</html>`))
}

// loginPost handles login form submission with proper credential verification.
func (h *Handlers) loginPost(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	next := r.FormValue("next")

	// Validate and sanitize the redirect URL to prevent open redirect
	next = sanitizeRedirectURL(next)

	// Check if we're in open mode (no password hashes configured)
	// In open mode, allow any login
	var accessKey string
	var ok bool
	if h.auth.IsOpenMode() {
		// Open mode - generate a session token
		var err error
		accessKey, err = auth.GenerateSessionToken()
		ok = err == nil
	} else {
		// Verify password using argon2
		accessKey, ok = h.auth.VerifyPassword(password)
	}
	if !ok {
		// Invalid password - show login page with error
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		safeNext := html.EscapeString(next)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>AmpliPi Login</title></head>
<body>
<h2>AmpliPi Login</h2>
<p style="color: red;">Invalid password. Please try again.</p>
<form method="POST" action="/auth/login">
  <input type="hidden" name="next" value="` + safeNext + `">
  <label>Password: <input type="password" name="password"></label>
  <button type="submit">Login</button>
</form>
</body>
</html>`))
		return
	}

	// Set secure session cookie
	// Determine if we should set Secure flag (only for HTTPS)
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	auth.SetSessionCookie(w, accessKey, secure)

	http.Redirect(w, r, next, http.StatusFound)
}

// sanitizeRedirectURL ensures the redirect URL is safe (relative path only).
// Prevents open redirect attacks by rejecting absolute URLs and paths starting with //.
func sanitizeRedirectURL(next string) string {
	if next == "" {
		return "/api"
	}

	// Reject absolute URLs (http://, https://, //, etc.)
	if strings.HasPrefix(next, "http://") ||
		strings.HasPrefix(next, "https://") ||
		strings.HasPrefix(next, "//") ||
		strings.Contains(next, "://") {
		return "/api"
	}

	// Ensure it starts with /
	if !strings.HasPrefix(next, "/") {
		return "/api"
	}

	// Reject paths that try to include a host via @ or other tricks
	if strings.Contains(next, "@") {
		return "/api"
	}

	return next
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
	configDir := h.configDir
	if configDir == "" {
		// Fallback to default config directory
		home, err := os.UserHomeDir()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "cannot determine home directory: " + err.Error(),
			})
			return
		}
		configDir = filepath.Join(home, ".config", "amplipi")
	}
	svc := maintenance.New(configDir, nil, nil)
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
// Only extracts regular files and directories, with proper path traversal prevention.
// Preserves directory structure while ensuring all paths stay within destDir.
func extractTarGz(r io.Reader, destDir string) error {
	// Get absolute path of destination for security checks
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("cannot resolve destination: %w", err)
	}

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

		// Skip symlinks and other special file types (security)
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}

		// Clean the path but preserve directory structure
		cleanName := filepath.Clean(hdr.Name)

		// Remove leading slashes and drive letters
		cleanName = strings.TrimPrefix(cleanName, "/")
		cleanName = strings.TrimPrefix(cleanName, "\\")

		// Skip empty names
		if cleanName == "" || cleanName == "." {
			continue
		}

		// Construct the destination path
		dest := filepath.Join(absDestDir, cleanName)

		// Verify the destination is within destDir (prevent path traversal)
		absDest, err := filepath.Abs(dest)
		if err != nil {
			continue // skip paths we can't resolve
		}
		if !strings.HasPrefix(absDest, absDestDir+string(filepath.Separator)) && absDest != absDestDir {
			continue // skip paths that escape the destination
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create parent directories if needed
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			// Create file with restrictive permissions
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}
			// Limit copy size to prevent decompression bombs (100MB per file)
			limitedReader := io.LimitReader(tr, 100<<20)
			if _, err := io.Copy(f, limitedReader); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
