package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/auth"
)

// newTempDir creates a temporary directory cleaned up by t.Cleanup.
func newTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "amplipi-auth-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// writeUsersJSON writes users.json to dir.
func writeUsersJSON(t *testing.T, dir string, users map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(users)
	if err != nil {
		t.Fatalf("json.Marshal users: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.json"), data, 0644); err != nil {
		t.Fatalf("WriteFile users.json: %v", err)
	}
}

// --- Open mode (no users.json) ---

func TestService_OpenMode_IsOpenMode(t *testing.T) {
	dir := newTempDir(t)
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	if !svc.IsOpenMode() {
		t.Error("IsOpenMode() = false, want true when no users.json")
	}
}

func TestService_OpenMode_VerifyKeyEmpty(t *testing.T) {
	dir := newTempDir(t)
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	// In open mode with no users, VerifyKey("") returns false (empty key always rejected)
	if svc.VerifyKey("") {
		t.Error("VerifyKey(\"\") = true, want false (empty key always rejected)")
	}
}

func TestService_OpenMode_VerifyKeyAny(t *testing.T) {
	dir := newTempDir(t)
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	// In open mode with no users, VerifyKey returns false for any key (no users to match)
	if svc.VerifyKey("any-key-at-all") {
		t.Error("VerifyKey(\"any-key\") = true in open mode with no users, want false")
	}
}

func TestMiddleware_OpenMode_PassesThrough(t *testing.T) {
	dir := newTempDir(t)
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := svc.Middleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("middleware in open mode did not call next handler")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("response code = %d, want 200", rr.Code)
	}
}

func TestMiddleware_OpenMode_NoCredentials_PassesThrough(t *testing.T) {
	dir := newTempDir(t)
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	called := false
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/zones", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("open-mode middleware blocked a request with no credentials")
	}
}

// --- Secured mode (users.json with access_key and password_hash) ---

func newSecuredService(t *testing.T, accessKey string) *auth.Service {
	t.Helper()
	dir := newTempDir(t)
	writeUsersJSON(t, dir, map[string]interface{}{
		"admin": map[string]interface{}{
			"type":          "admin",
			"access_key":    accessKey,
			"password_hash": "$argon2id$v=19$m=4096,t=3,p=1$fake$hash",
		},
	})

	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func TestService_SecuredMode_IsOpenMode_False(t *testing.T) {
	svc := newSecuredService(t, "secret-key-123")
	if svc.IsOpenMode() {
		t.Error("IsOpenMode() = true for service with password_hash, want false")
	}
}

func TestService_SecuredMode_VerifyCorrectKey(t *testing.T) {
	const key = "my-super-secret-key"
	svc := newSecuredService(t, key)

	if !svc.VerifyKey(key) {
		t.Errorf("VerifyKey(%q) = false, want true", key)
	}
}

func TestService_SecuredMode_VerifyWrongKey(t *testing.T) {
	svc := newSecuredService(t, "correct-key")

	if svc.VerifyKey("wrong-key") {
		t.Error("VerifyKey(\"wrong-key\") = true, want false")
	}
}

func TestService_SecuredMode_VerifyEmptyKey(t *testing.T) {
	svc := newSecuredService(t, "correct-key")

	if svc.VerifyKey("") {
		t.Error("VerifyKey(\"\") = true, want false (empty key always rejected)")
	}
}

func TestMiddleware_SecuredMode_APIKeyQueryParam_Passes(t *testing.T) {
	const key = "query-param-key"
	svc := newSecuredService(t, key)

	called := false
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api?api-key="+key, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("middleware did not pass request with correct api-key query param")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestMiddleware_SecuredMode_Cookie_Passes(t *testing.T) {
	const key = "cookie-session-key"
	svc := newSecuredService(t, key)

	called := false
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.AddCookie(&http.Cookie{Name: "amplipi-session", Value: key})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("middleware did not pass request with correct session cookie")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestMiddleware_SecuredMode_WrongKey_Redirects(t *testing.T) {
	svc := newSecuredService(t, "correct-key")

	called := false
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.AddCookie(&http.Cookie{Name: "amplipi-session", Value: "wrong-key"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("middleware called next handler despite wrong key")
	}
	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 (redirect to login)", rr.Code)
	}
	location := rr.Header().Get("Location")
	if location == "" {
		t.Error("expected Location header for redirect")
	}
}

func TestMiddleware_SecuredMode_NoCredentials_Redirects(t *testing.T) {
	svc := newSecuredService(t, "some-key")

	called := false
	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/zones", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called {
		t.Error("middleware called next handler despite no credentials")
	}
	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 (redirect to login)", rr.Code)
	}
}

func TestService_Reload(t *testing.T) {
	dir := newTempDir(t)

	// Start with no users.json
	svc, err := auth.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)

	if !svc.IsOpenMode() {
		t.Error("initially expected open mode")
	}

	// Write users.json with a password hash
	writeUsersJSON(t, dir, map[string]interface{}{
		"admin": map[string]interface{}{
			"type":          "admin",
			"access_key":    "reload-test-key",
			"password_hash": "somehash",
		},
	})

	// Manually reload
	if err := svc.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if svc.IsOpenMode() {
		t.Error("expected secured mode after reload with password_hash user")
	}
	if !svc.VerifyKey("reload-test-key") {
		t.Error("VerifyKey after reload returned false for correct key")
	}
}

func TestService_MissingConfigDir_NoError(t *testing.T) {
	// A non-existent directory should not cause an error —
	// it just means no users.json, which is open mode.
	dir := newTempDir(t)
	nonExistent := filepath.Join(dir, "does-not-exist")

	svc, err := auth.NewService(nonExistent)
	if err != nil {
		t.Fatalf("NewService with non-existent dir: %v", err)
	}
	t.Cleanup(svc.Close)

	if !svc.IsOpenMode() {
		t.Error("expected open mode for non-existent config dir")
	}
}
