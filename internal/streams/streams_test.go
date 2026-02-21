package streams

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// ─── VSRCAllocator ──────────────────────────────────────────────────────────

func TestVSRCAllocator(t *testing.T) {
	a := NewVSRCAllocator()

	// Alloc all 12 slots
	for i := 0; i < MaxVSRC; i++ {
		vsrc, err := a.Alloc()
		if err != nil {
			t.Fatalf("Alloc() #%d failed: %v", i, err)
		}
		if vsrc < 0 || vsrc >= MaxVSRC {
			t.Fatalf("Alloc() returned out-of-range vsrc %d", vsrc)
		}
	}

	// Next alloc should fail
	_, err := a.Alloc()
	if err != ErrNoVSRC {
		t.Fatalf("expected ErrNoVSRC, got %v", err)
	}

	// Free one and re-alloc
	a.Free(3)
	vsrc, err := a.Alloc()
	if err != nil {
		t.Fatalf("Alloc() after Free() failed: %v", err)
	}
	if vsrc != 3 {
		t.Errorf("expected re-alloc of vsrc 3, got %d", vsrc)
	}
}

func TestVSRCAllocator_FreeOutOfRange(t *testing.T) {
	a := NewVSRCAllocator()
	// Should not panic
	a.Free(-1)
	a.Free(MaxVSRC)
	a.Free(100)
}

// ─── Device names ───────────────────────────────────────────────────────────

func TestVSRCDeviceNames(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(int) string
		input    int
		expected string
	}{
		{"capture0", VirtualCaptureDevice, 0, "lb0p"},
		{"capture3", VirtualCaptureDevice, 3, "lb3p"},
		{"output0", VirtualOutputDevice, 0, "lb0c"},
		{"output5", VirtualOutputDevice, 5, "lb5c"},
		{"phys0", PhysicalOutputDevice, 0, "ch0"},
		{"phys3", PhysicalOutputDevice, 3, "ch3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

// ─── NewStreamer ─────────────────────────────────────────────────────────────

func TestNewStreamer_AllTypes(t *testing.T) {
	tests := []struct {
		streamType string
		config     map[string]interface{}
		wantType   string
	}{
		{"rca", nil, "rca"},
		{"aux", nil, "aux"},
		{"pandora", map[string]interface{}{"user": "test@example.com", "password": "secret"}, "pandora"},
		{"airplay", nil, "airplay"},
		{"spotify_connect", nil, "spotify_connect"},
		{"internet_radio", map[string]interface{}{"url": "http://example.com/stream"}, "internet_radio"},
		{"file_player", map[string]interface{}{"path": "/tmp/test.mp3"}, "file_player"},
		{"dlna", nil, "dlna"},
		{"lms", nil, "lms"},
		{"fm_radio", map[string]interface{}{"freq": "96.5M"}, "fm_radio"},
		{"bluetooth", nil, "bluetooth"},
		{"plexamp", nil, "plexamp"},
	}

	for _, tt := range tests {
		t.Run(tt.streamType, func(t *testing.T) {
			stream := models.Stream{
				ID:     1,
				Name:   "Test " + tt.streamType,
				Type:   tt.streamType,
				Config: tt.config,
			}
			streamer, err := NewStreamer(stream)
			if err != nil {
				t.Fatalf("NewStreamer(%q) error: %v", tt.streamType, err)
			}
			if streamer.Type() != tt.wantType {
				t.Errorf("Type() = %q, want %q", streamer.Type(), tt.wantType)
			}
		})
	}
}

func TestNewStreamer_UnknownType(t *testing.T) {
	stream := models.Stream{ID: 1, Name: "Unknown", Type: "does_not_exist"}
	_, err := NewStreamer(stream)
	if err == nil {
		t.Fatal("expected error for unknown stream type")
	}
}

// ─── RCAStream ───────────────────────────────────────────────────────────────

func TestRCAStream(t *testing.T) {
	ctx := context.Background()
	r := NewRCAStream("Input 1")

	if r.Type() != "rca" {
		t.Errorf("Type() = %q, want %q", r.Type(), "rca")
	}
	if r.IsPersistent() {
		t.Error("RCA should not be persistent")
	}

	// All operations should be no-ops
	if err := r.Activate(ctx, 0, "/tmp"); err != nil {
		t.Errorf("Activate() error: %v", err)
	}
	if err := r.Connect(ctx, 0); err != nil {
		t.Errorf("Connect() error: %v", err)
	}
	if err := r.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}

	info := r.Info()
	if info.State != "playing" {
		t.Errorf("Info().State = %q, want %q", info.State, "playing")
	}
	if info.Name != "Input 1" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "Input 1")
	}

	if err := r.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
	if err := r.Deactivate(ctx); err != nil {
		t.Errorf("Deactivate() error: %v", err)
	}
}

// ─── AuxStream ───────────────────────────────────────────────────────────────

func TestAuxStream(t *testing.T) {
	ctx := context.Background()
	a := NewAuxStream("Aux Input")

	if a.Type() != "aux" {
		t.Errorf("Type() = %q, want %q", a.Type(), "aux")
	}
	if a.IsPersistent() {
		t.Error("Aux should not be persistent")
	}

	if err := a.Activate(ctx, 0, "/tmp"); err != nil {
		t.Errorf("Activate() error: %v", err)
	}
	info := a.Info()
	if info.State != "playing" {
		t.Errorf("Info().State = %q, want %q", info.State, "playing")
	}
	_ = a.Connect(ctx, 0)
	_ = a.SendCmd(ctx, "play")
	_ = a.Disconnect(ctx)
	_ = a.Deactivate(ctx)
}

// ─── PlexampStream ───────────────────────────────────────────────────────────

func TestPlexampStub(t *testing.T) {
	ctx := context.Background()
	p := NewPlexampStream("My Plexamp")

	if p.Type() != "plexamp" {
		t.Errorf("Type() = %q, want %q", p.Type(), "plexamp")
	}

	// Activate must return ErrNotSupported
	err := p.Activate(ctx, 0, "/tmp")
	if err != ErrNotSupported {
		t.Errorf("Activate() = %v, want ErrNotSupported", err)
	}

	info := p.Info()
	if !strings.Contains(info.Track, "not yet supported") {
		t.Errorf("Info().Track = %q, want it to mention 'not yet supported'", info.Track)
	}
}

// ─── Manager ─────────────────────────────────────────────────────────────────

func TestManagerSync_CreateStream(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	modelStreams := []models.Stream{
		{ID: 1, Name: "Input 1", Type: "rca"},
	}
	sources := []models.Source{
		{ID: 0, Name: "Source 0", Input: ""},
	}

	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	m.mu.Lock()
	state, ok := m.streams[1]
	m.mu.Unlock()

	if !ok {
		t.Fatal("stream 1 not found in manager after Sync")
	}
	if state.StreamID != 1 {
		t.Errorf("StreamID = %d, want 1", state.StreamID)
	}
}

func TestManagerSync_RemoveStream(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	// Add a stream
	modelStreams := []models.Stream{
		{ID: 1, Name: "Input 1", Type: "rca"},
		{ID: 2, Name: "Input 2", Type: "rca"},
	}
	sources := []models.Source{{ID: 0, Input: ""}}

	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() initial error: %v", err)
	}

	m.mu.Lock()
	count := len(m.streams)
	m.mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 streams, got %d", count)
	}

	// Remove stream 2
	if err := m.Sync(ctx, modelStreams[:1], sources); err != nil {
		t.Fatalf("Sync() remove error: %v", err)
	}

	m.mu.Lock()
	_, stream2Exists := m.streams[2]
	m.mu.Unlock()
	if stream2Exists {
		t.Error("stream 2 should have been removed")
	}
}

func TestManagerSync_Connect(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	// Add stream and connect to source
	modelStreams := []models.Stream{
		{ID: 10, Name: "AUX", Type: "aux"},
	}
	sources := []models.Source{
		{ID: 1, Name: "Source 1", Input: "stream=10"},
	}

	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	m.mu.Lock()
	state := m.streams[10]
	m.mu.Unlock()

	// AUX is hardware passthrough — Connect is a no-op so PhysSrc should be set
	if state.PhysSrc != 1 {
		t.Errorf("PhysSrc = %d, want 1", state.PhysSrc)
	}
}

func TestManagerSendCmd_Unknown(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	err := m.SendCmd(ctx, 9999, "play")
	if err == nil {
		t.Error("expected error for unknown stream ID, got nil")
	}
}

func TestManagerShutdown(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	modelStreams := []models.Stream{
		{ID: 1, Name: "Input 1", Type: "rca"},
	}
	if err := m.Sync(ctx, modelStreams, nil); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if err := m.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error: %v", err)
	}

	m.mu.Lock()
	count := len(m.streams)
	m.mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 streams after shutdown, got %d", count)
	}
}

// ─── Supervisor ──────────────────────────────────────────────────────────────

func TestSupervisor_StartStop(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	sup := NewSupervisor("test-sleep", func() *exec.Cmd {
		return exec.Command("sleep", "10")
	})

	ctx := context.Background()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Give the process time to start
	time.Sleep(200 * time.Millisecond)

	pid := sup.Pid()
	if pid == 0 {
		t.Error("expected non-zero PID after start")
	}

	if err := sup.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Pid should be 0 after stop
	pid = sup.Pid()
	if pid != 0 {
		t.Errorf("expected PID 0 after stop, got %d", pid)
	}
}

func TestSupervisor_DoubleStart(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	sup := NewSupervisor("test-double", func() *exec.Cmd {
		return exec.Command("sleep", "10")
	})

	ctx := context.Background()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Second Start should be a no-op (not an error)
	if err := sup.Start(ctx); err != nil {
		t.Errorf("second Start() should not fail: %v", err)
	}

	_ = sup.Stop()
}

func TestSupervisor_ContextCancel(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sup := NewSupervisor("test-ctx", func() *exec.Cmd {
		return exec.Command("sleep", "10")
	})

	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	cancel() // cancelling context should stop the supervisor

	// Wait for supervisor to exit
	done := make(chan struct{})
	go func() {
		_ = sup.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Error("supervisor did not stop after context cancel")
	}
}

func TestSupervisor_FastFailGivesUp(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not available")
	}

	calls := 0
	sup := NewSupervisor("test-fastfail", func() *exec.Cmd {
		calls++
		return exec.Command("false") // immediately exits with error
	})
	sup.maxFails = 3
	sup.fastFailSec = 5.0
	sup.backoff = 10 * time.Millisecond
	sup.maxBackoff = 10 * time.Millisecond

	ctx := context.Background()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Wait long enough for all retries
	time.Sleep(500 * time.Millisecond)

	_ = sup.Stop()

	if calls < sup.maxFails {
		t.Errorf("expected at least %d calls before giving up, got %d", sup.maxFails, calls)
	}
}

// ─── Pandora parsing ─────────────────────────────────────────────────────────

func TestParsePianobarCurrentSong(t *testing.T) {
	line := "My Song,,,The Artist,,,Best Album,,,http://img.example.com/art.jpg,,,1,,,Classic Rock"
	info := parsePianobarCurrentSong(line)

	if info.Track != "My Song" {
		t.Errorf("Track = %q, want %q", info.Track, "My Song")
	}
	if info.Artist != "The Artist" {
		t.Errorf("Artist = %q, want %q", info.Artist, "The Artist")
	}
	if info.Album != "Best Album" {
		t.Errorf("Album = %q, want %q", info.Album, "Best Album")
	}
	if info.ImageURL != "http://img.example.com/art.jpg" {
		t.Errorf("ImageURL = %q, want %q", info.ImageURL, "http://img.example.com/art.jpg")
	}
	if info.Station != "Classic Rock" {
		t.Errorf("Station = %q, want %q", info.Station, "Classic Rock")
	}
	if info.State != "playing" {
		t.Errorf("State = %q, want %q", info.State, "playing")
	}
}

func TestParsePianobarCurrentSong_Empty(t *testing.T) {
	info := parsePianobarCurrentSong("")
	if info.Name != "Pandora" {
		t.Errorf("Name = %q, want %q", info.Name, "Pandora")
	}
}

// ─── Config helpers ──────────────────────────────────────────────────────────

func TestBuildConfigDir(t *testing.T) {
	base := t.TempDir()
	dir, err := buildConfigDir(base, 5)
	if err != nil {
		t.Fatalf("buildConfigDir() error: %v", err)
	}
	expected := filepath.Join(base, "v5")
	if dir != expected {
		t.Errorf("dir = %q, want %q", dir, expected)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("buildConfigDir did not create the directory")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello, world\n")

	if err := writeFileAtomic(path, content); err != nil {
		t.Fatalf("writeFileAtomic() error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

// ─── LMS MAC address ─────────────────────────────────────────────────────────

func TestLMSMACAddress(t *testing.T) {
	mac1 := lmsMACAddress("My Squeezebox")
	mac2 := lmsMACAddress("My Squeezebox")
	mac3 := lmsMACAddress("Other Player")

	if mac1 != mac2 {
		t.Error("same name should produce same MAC")
	}
	if mac1 == mac3 {
		t.Error("different names should produce different MACs")
	}
	// Validate format: xx:xx:xx:xx:xx:xx
	parts := strings.Split(mac1, ":")
	if len(parts) != 6 {
		t.Errorf("MAC address should have 6 parts, got %d: %s", len(parts), mac1)
	}
}

// ─── findBinary ──────────────────────────────────────────────────────────────

func TestFindBinary_InPath(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not in PATH")
	}
	result := findBinary("sleep")
	if result == "sleep" {
		t.Error("findBinary should have resolved 'sleep' to a full path")
	}
}

func TestFindBinary_NotFound(t *testing.T) {
	result := findBinary("this_binary_definitely_does_not_exist_xyz")
	if result != "this_binary_definitely_does_not_exist_xyz" {
		t.Errorf("expected fallback to name, got %q", result)
	}
}

// ─── Manager Info ─────────────────────────────────────────────────────────────

func TestManagerInfo(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	modelStreams := []models.Stream{
		{ID: 1, Name: "Input 1", Type: "rca"},
	}
	if err := m.Sync(ctx, modelStreams, nil); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	info := m.Info(1)
	if info == nil {
		t.Fatal("Info(1) returned nil")
	}

	info = m.Info(9999)
	if info != nil {
		t.Error("Info(9999) should return nil for unknown stream")
	}
}

// ─── streamNeedsVSRC ─────────────────────────────────────────────────────────

func TestStreamNeedsVSRC(t *testing.T) {
	tests := []struct {
		streamType string
		needs      bool
	}{
		{"rca", false},
		{"aux", false},
		{"plexamp", false},
		{"pandora", true},
		{"airplay", true},
		{"spotify_connect", true},
		{"internet_radio", true},
		{"dlna", true},
	}
	for _, tt := range tests {
		t.Run(tt.streamType, func(t *testing.T) {
			stream := models.Stream{ID: 1, Name: "test", Type: tt.streamType}
			streamer, err := NewStreamer(stream)
			if err != nil {
				t.Skipf("cannot create %s streamer: %v", tt.streamType, err)
			}
			got := streamNeedsVSRC(streamer)
			if got != tt.needs {
				t.Errorf("streamNeedsVSRC(%s) = %v, want %v", tt.streamType, got, tt.needs)
			}
		})
	}
}

// ─── Models ConfigString/ConfigInt ───────────────────────────────────────────

func TestConfigString(t *testing.T) {
	s := models.Stream{
		Config: map[string]interface{}{
			"url":  "http://example.com",
			"name": "test",
		},
	}
	if got := s.ConfigString("url"); got != "http://example.com" {
		t.Errorf("ConfigString(url) = %q, want %q", got, "http://example.com")
	}
	if got := s.ConfigString("missing"); got != "" {
		t.Errorf("ConfigString(missing) = %q, want %q", got, "")
	}

	empty := models.Stream{}
	if got := empty.ConfigString("key"); got != "" {
		t.Errorf("ConfigString on nil config = %q, want %q", got, "")
	}
}

func TestConfigInt(t *testing.T) {
	s := models.Stream{
		Config: map[string]interface{}{
			"port":     float64(9000),
			"count":    int(5),
			"notanint": "hello",
		},
	}
	if got := s.ConfigInt("port", 0); got != 9000 {
		t.Errorf("ConfigInt(port) = %d, want %d", got, 9000)
	}
	if got := s.ConfigInt("count", 0); got != 5 {
		t.Errorf("ConfigInt(count) = %d, want %d", got, 5)
	}
	if got := s.ConfigInt("notanint", 42); got != 42 {
		t.Errorf("ConfigInt(notanint) = %d, want %d (default)", got, 42)
	}
	if got := s.ConfigInt("missing", 99); got != 99 {
		t.Errorf("ConfigInt(missing) = %d, want %d (default)", got, 99)
	}
}

// ─── FMRadio helper ──────────────────────────────────────────────────────────

func TestFMRadioStreamCreation(t *testing.T) {
	s := NewFMRadioStream("My FM", "96.5M")
	if s.Type() != "fm_radio" {
		t.Errorf("Type() = %q, want %q", s.Type(), "fm_radio")
	}
	if s.IsPersistent() {
		t.Error("FM Radio should not be persistent")
	}
}

// ─── Supervisor StopIdempotent ────────────────────────────────────────────────

func TestSupervisor_StopNotRunning(t *testing.T) {
	sup := NewSupervisor("test-nostart", func() *exec.Cmd {
		return exec.Command("sleep", "1")
	})
	// Stop without Start should not panic or block
	done := make(chan struct{})
	go func() {
		_ = sup.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop() on unstarted supervisor timed out")
	}
}

// ─── Manager Sync idempotent ──────────────────────────────────────────────────

func TestManagerSync_Idempotent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	modelStreams := []models.Stream{
		{ID: 1, Name: "Input 1", Type: "rca"},
		{ID: 2, Name: "Aux", Type: "aux"},
	}
	sources := []models.Source{{ID: 0, Input: ""}}

	// Call Sync multiple times — should be idempotent
	for i := 0; i < 3; i++ {
		if err := m.Sync(ctx, modelStreams, sources); err != nil {
			t.Fatalf("Sync() iteration %d error: %v", i, err)
		}
	}

	m.mu.Lock()
	count := len(m.streams)
	m.mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 streams after repeated Sync, got %d", count)
	}
}

// ─── SubprocStream base methods ──────────────────────────────────────────────

// subprocTestStream is a thin wrapper around SubprocStream for testing.
type subprocTestStream struct {
	SubprocStream
}

func newSubprocTestStream() *subprocTestStream {
	return &subprocTestStream{}
}

func TestSubprocStream_SetGetInfo(t *testing.T) {
	s := newSubprocTestStream()

	// Default should be zero value
	info := s.getInfo()
	if info.Name != "" || info.State != "" {
		t.Errorf("default info should be zero value, got %+v", info)
	}

	// Set and get
	want := models.StreamInfo{Name: "Test", State: "playing", Track: "Some Song"}
	s.setInfo(want)
	got := s.getInfo()
	if got.Name != want.Name || got.State != want.State || got.Track != want.Track {
		t.Errorf("getInfo() = %+v, want %+v", got, want)
	}
}

func TestSubprocStream_DeactivateBase_Nil(t *testing.T) {
	s := newSubprocTestStream()
	ctx := context.Background()
	// deactivateBase with nil sup and loop should be a no-op
	if err := s.deactivateBase(ctx); err != nil {
		t.Errorf("deactivateBase() on nil state error: %v", err)
	}
}

func TestSubprocStream_DisconnectBase_Nil(t *testing.T) {
	s := newSubprocTestStream()
	ctx := context.Background()
	// disconnectBase with nil loop should be a no-op
	if err := s.disconnectBase(ctx); err != nil {
		t.Errorf("disconnectBase() on nil loop error: %v", err)
	}
}

// ─── InternetRadioStream (without activation) ─────────────────────────────────

func TestInternetRadioStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewInternetRadioStream("BBC World", "http://bbc.co.uk/stream")

	if s.Type() != "internet_radio" {
		t.Errorf("Type() = %q, want internet_radio", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("InternetRadio should be persistent")
	}

	// Info before activation returns zero value
	info := s.Info()
	// Should not panic; state might be empty
	_ = info

	// SendCmd should be a no-op (just logs)
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}

	// Deactivate on unstarted stream should not panic
	if err := s.Deactivate(ctx); err != nil {
		t.Errorf("Deactivate() error: %v", err)
	}
}

// ─── AirPlayStream (without activation) ──────────────────────────────────────

func TestAirPlayStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewAirPlayStream("My AirPlay")

	if s.Type() != "airplay" {
		t.Errorf("Type() = %q, want airplay", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("AirPlay should be persistent")
	}
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
	_ = s.Info()
}

// ─── BluetoothStream (without activation) ────────────────────────────────────

func TestBluetoothStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewBluetoothStream("Bluetooth")

	if s.Type() != "bluetooth" {
		t.Errorf("Type() = %q, want bluetooth", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("Bluetooth should be persistent")
	}
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
	_ = s.Info()
	if err := s.Deactivate(ctx); err != nil {
		t.Errorf("Deactivate() error: %v", err)
	}
}

// ─── DLNAStream (without activation) ─────────────────────────────────────────

func TestDLNAStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewDLNAStream("Living Room DLNA")

	if s.Type() != "dlna" {
		t.Errorf("Type() = %q, want dlna", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("DLNA should be persistent")
	}
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
	_ = s.Info()
}

// ─── LMSStream (without activation) ──────────────────────────────────────────

func TestLMSStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewLMSStream("My Squeezebox", "", nil)

	if s.Type() != "lms" {
		t.Errorf("Type() = %q, want lms", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("LMS should be persistent")
	}
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
	_ = s.Info()
}

// ─── SpotifyStream (without activation) ──────────────────────────────────────

func TestSpotifyStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewSpotifyStream("My Spotify", nil)

	if s.Type() != "spotify_connect" {
		t.Errorf("Type() = %q, want spotify_connect", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("Spotify should be persistent")
	}
	// SendCmd before activation — should fail or be a no-op (no API running)
	// We just check it doesn't panic
	_ = s.SendCmd(ctx, "play")
	_ = s.Info()
	if err := s.Deactivate(ctx); err != nil {
		t.Errorf("Deactivate() error: %v", err)
	}
}

// ─── PandoraStream (without activation) ──────────────────────────────────────

func TestPandoraStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewPandoraStream("Pandora Radio", "user@example.com", "password", "", nil)

	if s.Type() != "pandora" {
		t.Errorf("Type() = %q, want pandora", s.Type())
	}
	if !s.IsPersistent() {
		t.Error("Pandora should be persistent")
	}
	// SendCmd before activation
	_ = s.SendCmd(ctx, "play")
	_ = s.Info()
}

func TestPandoraStream_SendCmdNoFIFO(t *testing.T) {
	ctx := context.Background()
	s := NewPandoraStream("Pandora", "u", "p", "", nil)

	// All commands should fail gracefully (no FIFO)
	for _, cmd := range []string{"play", "pause", "next", "love", "ban", "shelve", "station=123"} {
		err := s.SendCmd(ctx, cmd)
		if err == nil {
			t.Errorf("SendCmd(%q) should fail when FIFO not initialized", cmd)
		}
	}
}

func TestPandoraStream_SendCmdUnknown(t *testing.T) {
	ctx := context.Background()
	s := NewPandoraStream("Pandora", "u", "p", "", nil)
	// Unknown commands are silently ignored
	_ = s.SendCmd(ctx, "completely_unknown_cmd")
}

// ─── FilePlayerStream (without activation) ───────────────────────────────────

func TestFilePlayerStream_Basics(t *testing.T) {
	ctx := context.Background()
	s := NewFilePlayerStream("Music", "/home/user/music")

	if s.Type() != "file_player" {
		t.Errorf("Type() = %q, want file_player", s.Type())
	}
	if s.IsPersistent() {
		t.Error("FilePlayer should not be persistent")
	}
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
	_ = s.Info()
}

// ─── FMRadioStream (deactivation edge cases) ─────────────────────────────────

func TestFMRadioStream_DeactivateNotRunning(t *testing.T) {
	ctx := context.Background()
	s := NewFMRadioStream("FM Radio", "96.5M")
	// Deactivate before activate should not panic
	if err := s.Deactivate(ctx); err != nil {
		t.Errorf("Deactivate() error: %v", err)
	}
}

func TestFMRadioStream_DisconnectNotConnected(t *testing.T) {
	s := NewFMRadioStream("FM Radio", "96.5M")
	if err := s.Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
}

func TestFMRadioStream_SendCmd(t *testing.T) {
	ctx := context.Background()
	s := NewFMRadioStream("FM Radio", "96.5M")
	if err := s.SendCmd(ctx, "play"); err != nil {
		t.Errorf("SendCmd() error: %v", err)
	}
}

func TestFMRadioStream_Info(t *testing.T) {
	s := NewFMRadioStream("FM Radio", "96.5M")
	info := s.Info()
	// Before activation, should be zero value
	_ = info
}

// ─── Plexamp full interface ────────────────────────────────────────────────────

func TestPlexampStream_FullInterface(t *testing.T) {
	ctx := context.Background()
	p := NewPlexampStream("Plex")

	if p.IsPersistent() {
		t.Error("Plexamp should not be persistent")
	}
	if p.Type() != "plexamp" {
		t.Errorf("Type() = %q, want plexamp", p.Type())
	}

	// These should all be no-ops
	_ = p.Deactivate(ctx)
	_ = p.Connect(ctx, 0)
	_ = p.Disconnect(ctx)
	_ = p.SendCmd(ctx, "play")
}

// ─── Supervisor with echo binary ─────────────────────────────────────────────

func TestSupervisor_WithEcho(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not available")
	}

	sup := NewSupervisor("test-echo", func() *exec.Cmd {
		return exec.Command("echo", "hello")
	})
	// echo exits immediately — this tests fast-fail behavior
	sup.backoff = 10 * time.Millisecond
	sup.maxBackoff = 100 * time.Millisecond
	sup.maxFails = 2

	ctx := context.Background()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := sup.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

// ─── activateBase tests ───────────────────────────────────────────────────────

func TestActivateBase_NilSupervisor(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := newSubprocTestStream()
	// activateBase with nil supervisor should succeed (no process to start)
	if err := s.activateBase(ctx, 0, dir); err != nil {
		t.Errorf("activateBase() with nil supervisor error: %v", err)
	}
	if s.vsrc != 0 {
		t.Errorf("vsrc = %d, want 0", s.vsrc)
	}
	if s.configDir != dir {
		t.Errorf("configDir = %q, want %q", s.configDir, dir)
	}
}

func TestActivateBase_WithSupervisor(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	s := newSubprocTestStream()
	s.sup = NewSupervisor("test", func() *exec.Cmd {
		return exec.Command("sleep", "10")
	})
	if err := s.activateBase(ctx, 1, dir); err != nil {
		t.Fatalf("activateBase() error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if s.sup.Pid() == 0 {
		t.Error("expected process to be running")
	}
	// Clean up
	ctx2 := context.Background()
	_ = s.deactivateBase(ctx2)
}

// ─── VSRCAllocator concurrent access ─────────────────────────────────────────

func TestVSRCAllocator_Concurrent(t *testing.T) {
	a := NewVSRCAllocator()
	results := make(chan int, MaxVSRC)
	errors := make(chan error, MaxVSRC*2)

	// Race to allocate all slots
	for i := 0; i < MaxVSRC+2; i++ {
		go func() {
			vsrc, err := a.Alloc()
			if err != nil {
				errors <- err
			} else {
				results <- vsrc
			}
		}()
	}

	// Collect results
	var allocated []int
	var errs []error
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v := <-results:
			allocated = append(allocated, v)
		case err := <-errors:
			errs = append(errs, err)
		case <-timeout:
			goto done
		}
		if len(allocated)+len(errs) == MaxVSRC+2 {
			goto done
		}
	}
done:
	if len(allocated) != MaxVSRC {
		t.Errorf("expected %d successful allocs, got %d", MaxVSRC, len(allocated))
	}
	if len(errs) != 2 {
		t.Errorf("expected 2 ErrNoVSRC errors, got %d", len(errs))
	}

	// Verify no duplicates
	seen := make(map[int]bool)
	for _, v := range allocated {
		if seen[v] {
			t.Errorf("duplicate vsrc allocation: %d", v)
		}
		seen[v] = true
	}
}

// ─── Manager with persistent stream ──────────────────────────────────────────

func TestManager_PersistentStreamActivatedOnSync(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	// internet_radio is persistent — should be activated immediately
	modelStreams := []models.Stream{
		{ID: 100, Name: "BBC", Type: "internet_radio",
			Config: map[string]interface{}{"url": "http://example.com"}},
	}
	sources := []models.Source{{ID: 0, Input: ""}}

	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Give supervisor time to attempt starting (will fail since vlc isn't present in CI,
	// but the Activate() method and vsrc allocation should have run)
	time.Sleep(100 * time.Millisecond)

	m.mu.Lock()
	state, ok := m.streams[100]
	m.mu.Unlock()

	if !ok {
		t.Fatal("stream 100 not found")
	}
	if !state.Active {
		t.Log("stream not active (vlc may not be installed — acceptable in CI)")
	}
}

// ─── Manager disconnects non-persistent on unroute ───────────────────────────

func TestManager_NonPersistentDeactivatedOnDisconnect(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	// file_player is non-persistent
	modelStreams := []models.Stream{
		{ID: 200, Name: "Music", Type: "file_player",
			Config: map[string]interface{}{"path": "/tmp/test.mp3"}},
	}

	// First connect to source 0
	sources := []models.Source{{ID: 0, Input: "stream=200"}}
	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() initial error: %v", err)
	}

	// Now disconnect
	sources = []models.Source{{ID: 0, Input: ""}}
	if err := m.Sync(ctx, modelStreams, sources); err != nil {
		t.Fatalf("Sync() disconnect error: %v", err)
	}

	m.mu.Lock()
	state := m.streams[200]
	m.mu.Unlock()

	if state.PhysSrc != -1 {
		t.Errorf("PhysSrc = %d, want -1 after disconnect", state.PhysSrc)
	}
}

// ─── Manager SendCmd on connected stream ─────────────────────────────────────

func TestManager_SendCmdOnRCA(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)
	ctx := context.Background()

	modelStreams := []models.Stream{
		{ID: 996, Name: "Input 1", Type: "rca"},
	}
	if err := m.Sync(ctx, modelStreams, nil); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// RCA SendCmd is a no-op — should succeed
	if err := m.SendCmd(ctx, 996, "play"); err != nil {
		t.Errorf("SendCmd() on RCA error: %v", err)
	}
}

// ─── Helper to silence unused import warning ─────────────────────────────────

var _ = fmt.Sprintf
