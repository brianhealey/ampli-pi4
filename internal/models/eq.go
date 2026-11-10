package models

import (
	"fmt"
	"math"
)

// Parametric EQ limits. They keep every band and the preamp inside ranges
// where a 16-bit, 48 kHz pipeline stays well behaved, and they are the only
// user-controlled values that reach a generated CamillaDSP config.
const (
	EQMaxBands    = 10
	EQMinFreq     = 20.0
	EQMaxFreq     = 20000.0
	EQMinGainDB   = -12.0
	EQMaxGainDB   = 12.0
	EQMinQ        = 0.1
	EQMaxQ        = 10.0
	EQMinPreampDB = -24.0
	EQMaxPreampDB = 0.0
)

// EQ band types.
const (
	EQBandPeaking   = "peaking"
	EQBandLowShelf  = "lowshelf"
	EQBandHighShelf = "highshelf"
)

// EQBand is one filter of a source's parametric EQ.
type EQBand struct {
	Type string  `json:"type"` // "peaking" | "lowshelf" | "highshelf"
	Freq float64 `json:"freq"` // Hz: centre (peaking) or corner (shelves)
	Gain float64 `json:"gain"` // dB
	Q    float64 `json:"q"`
}

// SourceEQ is the parametric EQ applied to whatever digital stream plays on a
// source. Absent or Enabled=false means the source is bridged exactly as it is
// without EQ.
type SourceEQ struct {
	Enabled  bool     `json:"enabled"`
	PreampDB float64  `json:"preamp_db"`
	Bands    []EQBand `json:"bands"`
}

// EQ status states reported in Source.EQStatus.
const (
	EQStateOff         = "off"         // EQ configured but disabled
	EQStateActive      = "active"      // CamillaDSP is processing this source
	EQStateStarting    = "starting"    // CamillaDSP is (re)starting
	EQStateIdle        = "idle"        // enabled, but no stream is selected on the source
	EQStateAnalog      = "analog"      // RCA/aux input: audio never reaches the Pi
	EQStateNoDAC       = "no_dac"      // source has no DAC, so no digital stream can play
	EQStateUnavailable = "unavailable" // CamillaDSP is not installed on this unit
	EQStateFailed      = "failed"      // CamillaDSP could not run; plain bridge in use
)

// EQStatus says whether a source's EQ is actually being applied, and why not.
// Derived at read time; never persisted and never set by clients.
type EQStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Clone returns a deep copy of the EQ (nil stays nil).
func (e *SourceEQ) Clone() *SourceEQ {
	if e == nil {
		return nil
	}
	c := *e
	if e.Bands != nil {
		c.Bands = make([]EQBand, len(e.Bands))
		copy(c.Bands, e.Bands)
	}
	return &c
}

// Validate checks the EQ against the limits above. The error text is meant
// for API clients (it becomes a 400 message).
func (e *SourceEQ) Validate() error {
	if e == nil {
		return nil
	}
	if !finite(e.PreampDB) || e.PreampDB < EQMinPreampDB || e.PreampDB > EQMaxPreampDB {
		return fmt.Errorf("eq.preamp_db must be between %g and %g dB", EQMinPreampDB, EQMaxPreampDB)
	}
	if len(e.Bands) > EQMaxBands {
		return fmt.Errorf("eq.bands: at most %d bands are allowed, got %d", EQMaxBands, len(e.Bands))
	}
	for i, b := range e.Bands {
		switch b.Type {
		case EQBandPeaking, EQBandLowShelf, EQBandHighShelf:
		default:
			return fmt.Errorf("eq.bands[%d].type must be one of %q, %q, %q, got %q",
				i, EQBandPeaking, EQBandLowShelf, EQBandHighShelf, b.Type)
		}
		if !finite(b.Freq) || b.Freq < EQMinFreq || b.Freq > EQMaxFreq {
			return fmt.Errorf("eq.bands[%d].freq must be between %g and %g Hz", i, EQMinFreq, EQMaxFreq)
		}
		if !finite(b.Gain) || b.Gain < EQMinGainDB || b.Gain > EQMaxGainDB {
			return fmt.Errorf("eq.bands[%d].gain must be between %g and %g dB", i, EQMinGainDB, EQMaxGainDB)
		}
		if !finite(b.Q) || b.Q < EQMinQ || b.Q > EQMaxQ {
			return fmt.Errorf("eq.bands[%d].q must be between %g and %g", i, EQMinQ, EQMaxQ)
		}
	}
	return nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
