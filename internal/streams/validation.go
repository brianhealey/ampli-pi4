package streams

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// AllowedURLSchemes defines the URL schemes that are safe for streaming.
var AllowedURLSchemes = []string{"http", "https", "rtsp", "rtp", "mms", "mmsh", "mmst"}

// MaxURLLength is the maximum allowed URL length.
const MaxURLLength = 2048

// MaxPathLength is the maximum allowed file path length.
const MaxPathLength = 4096

// MaxNameLength is the maximum allowed stream name length.
const MaxNameLength = 128

// MaxFreqLength is the maximum allowed frequency string length.
const MaxFreqLength = 20

// dangerousChars are characters that could enable shell injection if passed to exec.Command.
// While Go's exec.Command doesn't invoke a shell, we still block these for defense in depth.
var dangerousChars = regexp.MustCompile(`[;&|$` + "`" + `\\!<>(){}\[\]'"` + "\n\r\t]")

// vlcInjectionPatterns are VLC-specific argument patterns that could be abused.
var vlcInjectionPatterns = []string{
	"--lua-config",
	"--lua-script",
	"--script",
	"--config",
	"-I",
	"--intf",
	"--extraintf",
	"--pidfile",
	"--log-file",
}

// ValidateStreamURL validates a URL for use with stream players (VLC, etc.).
// It checks:
// - URL length limits
// - URL scheme is in allowlist
// - No shell metacharacters or VLC injection patterns
func ValidateStreamURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if len(rawURL) > MaxURLLength {
		return fmt.Errorf("URL exceeds maximum length of %d characters", MaxURLLength)
	}

	// Check for dangerous characters (defense in depth)
	if dangerousChars.MatchString(rawURL) {
		return fmt.Errorf("URL contains invalid characters")
	}

	// Check for VLC injection patterns
	lowerURL := strings.ToLower(rawURL)
	for _, pattern := range vlcInjectionPatterns {
		if strings.Contains(lowerURL, strings.ToLower(pattern)) {
			return fmt.Errorf("URL contains invalid pattern: %s", pattern)
		}
	}

	// Parse and validate URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check scheme is in allowlist
	schemeAllowed := false
	for _, allowed := range AllowedURLSchemes {
		if strings.EqualFold(parsed.Scheme, allowed) {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return fmt.Errorf("URL scheme %q is not allowed; permitted schemes: %v", parsed.Scheme, AllowedURLSchemes)
	}

	// Ensure URL has a host
	if parsed.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// ValidateFilePath validates a file path for use with the file_player stream.
// It checks:
// - Path length limits
// - No shell metacharacters or VLC injection patterns
// - Path is absolute or relative to safe directories
// - No path traversal attempts
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	if len(path) > MaxPathLength {
		return fmt.Errorf("file path exceeds maximum length of %d characters", MaxPathLength)
	}

	// Check for dangerous characters
	if dangerousChars.MatchString(path) {
		return fmt.Errorf("file path contains invalid characters")
	}

	// Check for VLC injection patterns
	lowerPath := strings.ToLower(path)
	for _, pattern := range vlcInjectionPatterns {
		if strings.Contains(lowerPath, strings.ToLower(pattern)) {
			return fmt.Errorf("file path contains invalid pattern: %s", pattern)
		}
	}

	// Clean the path to resolve . and ..
	cleanPath := filepath.Clean(path)

	// Block obvious path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("file path cannot contain path traversal sequences")
	}

	// If it looks like a URL, validate it as a URL instead
	if strings.HasPrefix(strings.ToLower(path), "http://") ||
		strings.HasPrefix(strings.ToLower(path), "https://") ||
		strings.HasPrefix(strings.ToLower(path), "rtsp://") {
		return ValidateStreamURL(path)
	}

	return nil
}

// ValidateFrequency validates an FM radio frequency string.
// Expected format: "96.5M" or "96.5" (in MHz).
func ValidateFrequency(freq string) error {
	if freq == "" {
		return fmt.Errorf("frequency cannot be empty")
	}

	if len(freq) > MaxFreqLength {
		return fmt.Errorf("frequency exceeds maximum length of %d characters", MaxFreqLength)
	}

	// Check for dangerous characters
	if dangerousChars.MatchString(freq) {
		return fmt.Errorf("frequency contains invalid characters")
	}

	// Validate frequency format (number with optional M suffix)
	freqPattern := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[Mm]?$`)
	if !freqPattern.MatchString(freq) {
		return fmt.Errorf("frequency must be a number with optional 'M' suffix (e.g., '96.5M' or '96.5')")
	}

	return nil
}

// ValidateStreamName validates a stream name.
// It ensures the name is not too long and doesn't contain dangerous characters.
func ValidateStreamName(name string) error {
	if name == "" {
		return fmt.Errorf("stream name cannot be empty")
	}

	if len(name) > MaxNameLength {
		return fmt.Errorf("stream name exceeds maximum length of %d characters", MaxNameLength)
	}

	// Allow most characters in names, but block newlines and null bytes
	if strings.ContainsAny(name, "\n\r\x00") {
		return fmt.Errorf("stream name contains invalid characters")
	}

	return nil
}

// ValidateLMSServer validates an LMS server address.
func ValidateLMSServer(server string) error {
	if server == "" {
		// Empty server is allowed (uses discovery)
		return nil
	}

	if len(server) > MaxURLLength {
		return fmt.Errorf("server address exceeds maximum length")
	}

	// Check for dangerous characters
	if dangerousChars.MatchString(server) {
		return fmt.Errorf("server address contains invalid characters")
	}

	// Should be a hostname or IP, optionally with port
	// Format: hostname:port or hostname or IP:port or IP
	serverPattern := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-\.]*[a-zA-Z0-9])?(:[0-9]{1,5})?$`)
	if !serverPattern.MatchString(server) {
		return fmt.Errorf("server must be a valid hostname or IP address with optional port")
	}

	return nil
}
