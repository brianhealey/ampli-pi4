package models

// HardwareStatus describes the detected preamp topology plus live telemetry
// read from each unit. It powers the diagnostics/about view.
type HardwareStatus struct {
	Detected        bool               `json:"detected"`   // a hardware profile was available
	Mock            bool               `json:"mock"`       // running against the mock driver
	UnitCount       int                `json:"unit_count"` // controllers found on the I2C bus
	TotalZones      int                `json:"total_zones"`
	TotalSources    int                `json:"total_sources"`
	IsStreamer      bool               `json:"is_streamer"`
	FanMode         string             `json:"fan_mode"`
	HV2Present      bool               `json:"hv2_present"`
	Display         string             `json:"display"`
	FirmwareVersion string             `json:"firmware_version"` // main unit, "Major.Minor-githash"
	PhysicalOutputs []int              `json:"physical_outputs"`
	Units           []UnitStatus       `json:"units"`
	StreamSupport   []StreamCapability `json:"stream_support"`
}

// UnitStatus is the identity and live state of a single preamp unit.
type UnitStatus struct {
	Index           int    `json:"index"` // 0 = main, 1-5 = expanders
	I2CAddress      string `json:"i2c_address"`
	Type            string `json:"type"` // main | expansion | streamer | unknown
	BoardRevision   string `json:"board_revision"`
	Serial          uint32 `json:"serial"`
	FirmwareVersion string `json:"firmware_version"`
	ZoneBase        int    `json:"zone_base"`
	ZoneCount       int    `json:"zone_count"`
	HasAnalog       bool   `json:"has_analog"`
	Rev4Plus        bool   `json:"rev4_plus"`
	Online          bool   `json:"online"` // responded to the telemetry read
	Error           string `json:"error,omitempty"`

	Temps *UnitTemps `json:"temps,omitempty"`
	Power *UnitPower `json:"power,omitempty"`
	Fans  *UnitFans  `json:"fans,omitempty"`
}

// UnitTemps holds a unit's temperature sensors in degrees Celsius.
type UnitTemps struct {
	Amp1C float32 `json:"amp1_c"` // zones 1-3 heatsink
	Amp2C float32 `json:"amp2_c"` // zones 4-6 heatsink
	PSU1C float32 `json:"psu1_c"` // HV1 supply
	PSU2C float32 `json:"psu2_c"` // HV2 supply
	PiC   float32 `json:"pi_c"`   // Pi CPU as reported back by firmware
}

// UnitPower holds a unit's power rail status flags.
type UnitPower struct {
	PG9V       bool `json:"pg_9v"`
	EN9V       bool `json:"en_9v"`
	PG12V      bool `json:"pg_12v"`
	EN12V      bool `json:"en_12v"`
	PG5VD      bool `json:"pg_5vd"`
	PG5VA      bool `json:"pg_5va"`
	HV2Present bool `json:"hv2_present"`
}

// UnitFans holds a unit's fan controller state.
type UnitFans struct {
	Control  string `json:"control"` // external | pwm | linear | forced
	On       bool   `json:"on"`
	OverTemp bool   `json:"over_temp"`
	Failed   bool   `json:"failed"`
}

// StreamCapability reports whether a stream type's backing binary is installed.
type StreamCapability struct {
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Binary    string `json:"binary,omitempty"`
	Reason    string `json:"reason,omitempty"`
}
