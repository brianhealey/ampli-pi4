package hardware

import (
	"context"
	"sync"
	"time"
)

// Mock is a thread-safe in-memory mock hardware driver for testing and development.
type Mock struct {
	mu        sync.Mutex
	regs      map[int]map[Register]byte // unit → register → value
	units     []int
	failWrite bool
	failRead  bool
}

// NewMock creates a new mock driver with unit 0 pre-initialized.
func NewMock() *Mock {
	m := &Mock{
		regs:  make(map[int]map[Register]byte),
		units: []int{0},
	}
	m.initUnit(0)
	return m
}

// NewMockWithUnits creates a mock driver with the specified units.
func NewMockWithUnits(units []int) *Mock {
	m := &Mock{
		regs:  make(map[int]map[Register]byte),
		units: units,
	}
	for _, u := range units {
		m.initUnit(u)
	}
	return m
}

func (m *Mock) initUnit(unit int) {
	regs := make(map[Register]byte)
	// Default: all zones muted, all amps enabled, sources digital
	regs[RegMute] = 0x3F   // all 6 zones muted
	regs[RegAmpEn] = 0x3F  // all 6 zones amp enabled
	regs[RegSrcAD] = 0x00  // all sources analog (0)
	for i := byte(0); i < 6; i++ {
		regs[RegVolZone1+i] = VolMuteReg // all zones at mute volume
	}
	m.regs[unit] = regs
}

// SetFailWrite configures the mock to fail all write operations.
func (m *Mock) SetFailWrite(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failWrite = fail
}

// SetFailRead configures the mock to fail all read operations.
func (m *Mock) SetFailRead(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failRead = fail
}

func (m *Mock) Init(ctx context.Context) error {
	return nil
}

func (m *Mock) Write(ctx context.Context, unit int, reg Register, val byte) error {
	// Simulate I2C timing
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	if _, ok := m.regs[unit]; !ok {
		m.regs[unit] = make(map[Register]byte)
	}
	m.regs[unit][reg] = val
	return nil
}

func (m *Mock) Read(ctx context.Context, unit int, reg Register) (byte, error) {
	// Simulate I2C timing
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead {
		return 0, ErrHardware("mock: read failure configured")
	}
	if regs, ok := m.regs[unit]; ok {
		if val, ok := regs[reg]; ok {
			return val, nil
		}
	}
	return 0, nil
}

func (m *Mock) SetSourceTypes(ctx context.Context, unit int, analog [4]bool) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	var val byte
	for i, a := range analog {
		if !a { // digital = bit set
			val |= 1 << uint(i)
		}
	}
	m.ensureUnit(unit)
	m.regs[unit][RegSrcAD] = val
	return nil
}

func (m *Mock) SetZoneSources(ctx context.Context, unit int, sources [6]int) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	m.regs[unit][RegZone321] = PackZone321(sources[0], sources[1], sources[2])
	m.regs[unit][RegZone654] = PackZone654(sources[3], sources[4], sources[5])
	return nil
}

func (m *Mock) SetZoneMutes(ctx context.Context, unit int, mutes [6]bool) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	var val byte
	for i, mu := range mutes {
		if mu {
			val |= 1 << uint(i)
		}
	}
	m.regs[unit][RegMute] = val
	return nil
}

func (m *Mock) SetAmpEnables(ctx context.Context, unit int, enables [6]bool) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	var val byte
	for i, en := range enables {
		if en {
			val |= 1 << uint(i)
		}
	}
	m.regs[unit][RegAmpEn] = val
	return nil
}

func (m *Mock) SetZoneVol(ctx context.Context, unit, zone int, vol int) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	if zone < 0 || zone > 5 {
		return ErrHardware("invalid zone index")
	}
	m.ensureUnit(unit)
	m.regs[unit][VolZoneReg(zone)] = DBToVolReg(vol)
	return nil
}

func (m *Mock) ReadTemps(ctx context.Context, unit int) (Temps, error) {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead {
		return Temps{}, ErrHardware("mock: read failure configured")
	}
	regs := m.getOrInit(unit)
	return Temps{
		Amp1C: TempFromReg(regs[RegAmpTemp1]),
		Amp2C: TempFromReg(regs[RegAmpTemp2]),
		PSU1C: TempFromReg(regs[RegHV1Temp]),
		PSU2C: TempFromReg(regs[RegHV2Temp]),
		PiC:   TempFromReg(regs[RegPiTemp]),
	}, nil
}

func (m *Mock) ReadPower(ctx context.Context, unit int) (Power, error) {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead {
		return Power{}, ErrHardware("mock: read failure configured")
	}
	// Return a reasonable default power state
	return Power{
		PG9V:  true,
		EN9V:  true,
		PG12V: true,
		EN12V: true,
		PG5VD: true,
		PG5VA: true,
	}, nil
}

func (m *Mock) ReadFanStatus(ctx context.Context, unit int) (FanStatus, error) {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead {
		return FanStatus{}, ErrHardware("mock: read failure configured")
	}
	return FanStatus{Ctrl: 0, On: false}, nil
}

func (m *Mock) WriteRPiTemp(ctx context.Context, unit int, tempC float32) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	m.regs[unit][RegPiTemp] = TempToReg(tempC)
	return nil
}

func (m *Mock) ReadVersion(ctx context.Context, unit int) (Version, error) {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead {
		return Version{}, ErrHardware("mock: read failure configured")
	}
	return Version{Major: 1, Minor: 0, GitHash: [4]byte{0xde, 0xad, 0xbe, 0xef}}, nil
}

func (m *Mock) SetLEDOverride(ctx context.Context, unit int, enable bool) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	if enable {
		m.regs[unit][RegLEDCtrl] = 1
	} else {
		m.regs[unit][RegLEDCtrl] = 0
	}
	return nil
}

func (m *Mock) SetLEDState(ctx context.Context, unit int, leds LEDState) error {
	time.Sleep(time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite {
		return ErrHardware("mock: write failure configured")
	}
	m.ensureUnit(unit)
	var val byte
	if leds.Green {
		val |= 1 << 0
	}
	if leds.Red {
		val |= 1 << 1
	}
	for i, on := range leds.Zones {
		if on {
			val |= 1 << uint(i+2)
		}
	}
	m.regs[unit][RegLEDVal] = val
	return nil
}

func (m *Mock) Units() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]int, len(m.units))
	copy(result, m.units)
	return result
}

func (m *Mock) IsReal() bool {
	return false
}

// GetReg returns a register value for testing purposes.
func (m *Mock) GetReg(unit int, reg Register) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if regs, ok := m.regs[unit]; ok {
		return regs[reg]
	}
	return 0
}

func (m *Mock) ensureUnit(unit int) {
	if _, ok := m.regs[unit]; !ok {
		m.initUnit(unit)
	}
}

func (m *Mock) getOrInit(unit int) map[Register]byte {
	if _, ok := m.regs[unit]; !ok {
		m.initUnit(unit)
	}
	return m.regs[unit]
}

// HardwareError is returned when a hardware operation fails.
type HardwareError struct {
	msg string
}

func (e HardwareError) Error() string { return e.msg }

// ErrHardware creates a new hardware error.
func ErrHardware(msg string) error { return HardwareError{msg: msg} }
