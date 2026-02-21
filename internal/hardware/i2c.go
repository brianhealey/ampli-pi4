//go:build linux

package hardware

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
)

// I2C device addresses for AmpliPi preamp units.
// Unit 0 (master) = 0x08 (7-bit), unit 1 = 0x10, etc. (each +0x08).
var devAddrs = []uint16{0x08, 0x10, 0x18, 0x20, 0x28, 0x30}

const (
	i2cDevPath   = "/dev/i2c-1"
	i2cSlave     = 0x0703 // I2C_SLAVE ioctl
	maxOpsPerSec = 500
)

// I2CDriver is the real hardware driver for the AmpliPi preamp board,
// communicating via Linux I2C ioctl.
type I2CDriver struct {
	mu      sync.Mutex
	fds     map[int]int // unit → file descriptor
	units   []int
	limiter *rate.Limiter
}

// NewI2C creates a new real I2C hardware driver.
func NewI2C() *I2CDriver {
	return &I2CDriver{
		fds:     make(map[int]int),
		limiter: rate.NewLimiter(rate.Limit(maxOpsPerSec), 10),
	}
}

func (d *I2CDriver) Init(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var detected []int
	for unit, addr := range devAddrs {
		fd, err := unix.Open(i2cDevPath, unix.O_RDWR, 0)
		if err != nil {
			continue
		}
		// Set I2C slave address
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), i2cSlave, uintptr(addr)); errno != 0 {
			unix.Close(fd)
			continue
		}
		// Probe: try to read version register
		probe := make([]byte, 1)
		if err := d.rawWrite(fd, []byte{RegVersionMaj}); err != nil {
			unix.Close(fd)
			continue
		}
		if _, err := unix.Read(fd, probe); err != nil {
			unix.Close(fd)
			continue
		}
		d.fds[unit] = fd
		detected = append(detected, unit)
	}

	if len(detected) == 0 {
		return fmt.Errorf("i2c: no AmpliPi preamp units detected on %s", i2cDevPath)
	}
	d.units = detected
	return nil
}

func (d *I2CDriver) Write(ctx context.Context, unit int, reg Register, val byte) error {
	if err := d.limiter.Wait(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fd, err := d.getFD(unit)
	if err != nil {
		return err
	}
	return d.rawWrite(fd, []byte{reg, val})
}

func (d *I2CDriver) Read(ctx context.Context, unit int, reg Register) (byte, error) {
	if err := d.limiter.Wait(ctx); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fd, err := d.getFD(unit)
	if err != nil {
		return 0, err
	}
	// Write register address
	if err := d.rawWrite(fd, []byte{reg}); err != nil {
		return 0, fmt.Errorf("i2c: write reg addr: %w", err)
	}
	// Read response byte
	buf := make([]byte, 1)
	if _, err := unix.Read(fd, buf); err != nil {
		return 0, fmt.Errorf("i2c: read: %w", err)
	}
	return buf[0], nil
}

func (d *I2CDriver) SetSourceTypes(ctx context.Context, unit int, analog [4]bool) error {
	var val byte
	for i, a := range analog {
		if !a { // digital = bit set
			val |= 1 << uint(i)
		}
	}
	return d.Write(ctx, unit, RegSrcAD, val)
}

func (d *I2CDriver) SetZoneSources(ctx context.Context, unit int, sources [6]int) error {
	r321 := PackZone321(sources[0], sources[1], sources[2])
	r654 := PackZone654(sources[3], sources[4], sources[5])
	if err := d.Write(ctx, unit, RegZone321, r321); err != nil {
		return err
	}
	return d.Write(ctx, unit, RegZone654, r654)
}

func (d *I2CDriver) SetZoneMutes(ctx context.Context, unit int, mutes [6]bool) error {
	var val byte
	for i, mu := range mutes {
		if mu {
			val |= 1 << uint(i)
		}
	}
	return d.Write(ctx, unit, RegMute, val)
}

func (d *I2CDriver) SetAmpEnables(ctx context.Context, unit int, enables [6]bool) error {
	var val byte
	for i, en := range enables {
		if en {
			val |= 1 << uint(i)
		}
	}
	return d.Write(ctx, unit, RegAmpEn, val)
}

func (d *I2CDriver) SetZoneVol(ctx context.Context, unit, zone int, vol int) error {
	if zone < 0 || zone > 5 {
		return fmt.Errorf("i2c: invalid local zone %d", zone)
	}
	return d.Write(ctx, unit, VolZoneReg(zone), DBToVolReg(vol))
}

func (d *I2CDriver) ReadTemps(ctx context.Context, unit int) (Temps, error) {
	amp1, err := d.Read(ctx, unit, RegAmpTemp1)
	if err != nil {
		return Temps{}, err
	}
	hv1, err := d.Read(ctx, unit, RegHV1Temp)
	if err != nil {
		return Temps{}, err
	}
	amp2, err := d.Read(ctx, unit, RegAmpTemp2)
	if err != nil {
		return Temps{}, err
	}
	pi, err := d.Read(ctx, unit, RegPiTemp)
	if err != nil {
		return Temps{}, err
	}
	hv2, err := d.Read(ctx, unit, RegHV2Temp)
	if err != nil {
		return Temps{}, err
	}
	return Temps{
		Amp1C: TempFromReg(amp1),
		Amp2C: TempFromReg(amp2),
		PSU1C: TempFromReg(hv1),
		PSU2C: TempFromReg(hv2),
		PiC:   TempFromReg(pi),
	}, nil
}

func (d *I2CDriver) ReadPower(ctx context.Context, unit int) (Power, error) {
	val, err := d.Read(ctx, unit, RegPower)
	if err != nil {
		return Power{}, err
	}
	return Power{
		PG9V:       val&(1<<0) != 0,
		EN9V:       val&(1<<1) != 0,
		PG12V:      val&(1<<2) != 0,
		EN12V:      val&(1<<3) != 0,
		PG5VD:      val&(1<<4) != 0,
		PG5VA:      val&(1<<5) != 0,
		HV2Present: val&(1<<6) != 0,
	}, nil
}

func (d *I2CDriver) ReadFanStatus(ctx context.Context, unit int) (FanStatus, error) {
	val, err := d.Read(ctx, unit, RegFans)
	if err != nil {
		return FanStatus{}, err
	}
	return FanStatus{
		Ctrl:   int(val & 0x03),
		On:     val&(1<<2) != 0,
		OvrTmp: val&(1<<3) != 0,
		Fail:   val&(1<<4) != 0,
	}, nil
}

func (d *I2CDriver) WriteRPiTemp(ctx context.Context, unit int, tempC float32) error {
	return d.Write(ctx, unit, RegPiTemp, TempToReg(tempC))
}

func (d *I2CDriver) ReadVersion(ctx context.Context, unit int) (Version, error) {
	maj, err := d.Read(ctx, unit, RegVersionMaj)
	if err != nil {
		return Version{}, err
	}
	min, err := d.Read(ctx, unit, RegVersionMin)
	if err != nil {
		return Version{}, err
	}
	h65, _ := d.Read(ctx, unit, RegGitHash65)
	h43, _ := d.Read(ctx, unit, RegGitHash43)
	h21, _ := d.Read(ctx, unit, RegGitHash21)
	h0d, _ := d.Read(ctx, unit, RegGitHash0D)
	return Version{
		Major:   int(maj),
		Minor:   int(min),
		GitHash: [4]byte{h65, h43, h21, h0d},
	}, nil
}

func (d *I2CDriver) SetLEDOverride(ctx context.Context, unit int, enable bool) error {
	var val byte
	if enable {
		val = 1
	}
	return d.Write(ctx, unit, RegLEDCtrl, val)
}

func (d *I2CDriver) SetLEDState(ctx context.Context, unit int, leds LEDState) error {
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
	return d.Write(ctx, unit, RegLEDVal, val)
}

func (d *I2CDriver) Units() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]int, len(d.units))
	copy(result, d.units)
	return result
}

func (d *I2CDriver) IsReal() bool { return true }

// Close closes all open file descriptors.
func (d *I2CDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, fd := range d.fds {
		unix.Close(fd)
	}
	d.fds = make(map[int]int)
}

func (d *I2CDriver) getFD(unit int) (int, error) {
	fd, ok := d.fds[unit]
	if !ok {
		return 0, fmt.Errorf("i2c: unit %d not initialized", unit)
	}
	return fd, nil
}

func (d *I2CDriver) rawWrite(fd int, data []byte) error {
	written := 0
	for written < len(data) {
		n, err := unix.Write(fd, data[written:])
		if err != nil {
			return fmt.Errorf("i2c: write: %w", err)
		}
		written += n
	}
	return nil
}
