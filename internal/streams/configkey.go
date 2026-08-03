package streams

import (
	"encoding/json"
	"fmt"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// streamConfigKey fingerprints the parts of a stream that are baked into its
// Streamer at construction.
//
// NewStreamer runs once, when the manager first sees a stream, so anything it
// reads from Config is fixed for the life of that Streamer. Editing the config
// would otherwise persist to disk and change nothing until a restart. Sync
// compares this key to decide when a Streamer must be rebuilt.
//
// json.Marshal sorts map keys, so the result does not depend on map iteration
// order.
func streamConfigKey(s models.Stream) string {
	cfg, err := json.Marshal(s.Config)
	if err != nil {
		// Unmarshalable config: fall back to a value that differs from every
		// well-formed key, so the stream rebuilds rather than silently sticking.
		return fmt.Sprintf("%s|unmarshalable:%v", s.Type, s.Config)
	}
	return s.Type + "|" + string(cfg)
}
