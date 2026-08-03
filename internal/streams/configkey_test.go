package streams

import (
	"testing"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// A streamer is built once, from the stream's config, and never rebuilt while
// it exists. Settings baked in at construction — go-librespot's bitrate, for
// one — therefore only take effect if the manager notices the config changed
// and recreates it. This key is what it compares.
func TestStreamConfigKey(t *testing.T) {
	base := models.Stream{ID: 1, Type: "spotify", Config: map[string]interface{}{"bitrate": 160}}

	t.Run("stable across calls", func(t *testing.T) {
		if streamConfigKey(base) != streamConfigKey(base) {
			t.Error("key is not stable for identical input")
		}
	})

	t.Run("changes when a config value changes", func(t *testing.T) {
		changed := models.Stream{ID: 1, Type: "spotify", Config: map[string]interface{}{"bitrate": 320}}
		if streamConfigKey(base) == streamConfigKey(changed) {
			t.Error("bitrate change did not change the key")
		}
	})

	t.Run("independent of map ordering", func(t *testing.T) {
		a := models.Stream{ID: 1, Type: "x", Config: map[string]interface{}{"a": 1, "b": 2}}
		b := models.Stream{ID: 1, Type: "x", Config: map[string]interface{}{"b": 2, "a": 1}}
		if streamConfigKey(a) != streamConfigKey(b) {
			t.Error("key depends on map iteration order")
		}
	})

	t.Run("changes when the type changes", func(t *testing.T) {
		other := models.Stream{ID: 1, Type: "airplay", Config: base.Config}
		if streamConfigKey(base) == streamConfigKey(other) {
			t.Error("type change did not change the key")
		}
	})

	t.Run("nil config is handled", func(t *testing.T) {
		none := models.Stream{ID: 1, Type: "spotify"}
		if streamConfigKey(none) == "" {
			t.Error("nil config produced an empty key")
		}
		if streamConfigKey(none) == streamConfigKey(base) {
			t.Error("nil config matched a populated one")
		}
	})
}
