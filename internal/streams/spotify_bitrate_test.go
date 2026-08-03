package streams

import "testing"

// go-librespot accepts only 96, 160 or 320 kbps and rejects the config
// otherwise, which would leave the stream dead rather than merely low quality.
// 320 needs Spotify Premium, so the default has to stay at go-librespot's own
// default of 160 — a free account cannot stream at 320.
func TestSpotifyBitrate(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"unset falls back to the safe default", 0, 160},
		{"low", 96, 96},
		{"normal", 160, 160},
		{"premium high", 320, 320},
		{"unsupported value", 256, 160},
		{"negative", -1, 160},
		{"absurd", 999999, 160},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SpotifyBitrate(tc.in); got != tc.want {
				t.Errorf("SpotifyBitrate(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
