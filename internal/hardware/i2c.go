//go:build linux

package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unsafe"

	"go.bug.st/serial"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
)

// I2C device addresses for AmpliPi preamp units.
// Unit 0 (master) = 0x08 (7-bit), unit 1 = 0x10, etc. (each +0x08).
var devAddrs = []uint16{0x08, 0x10, 0x18, 0x20, 0x28, 0x30}

const (
	i2cDevPath   = "/dev/i2c-1"
	i2cSlave     = 0x0703 // I2C_SLAVE ioctl
	i2cRdwrIOCTL = 0x0707 // I2C_RDWR ioctl — combined write+read with REPEATED START
	i2cMsgRD     = 0x0001 // i2c_msg flag: read direction
	maxOpsPerSec = 500
)

// i2cMsg mirrors struct i2c_msg from linux/i2c.h
type i2cMsg struct {
	addr   uint16
	flags  uint16
	length uint16
	_pad   uint16 // struct alignment
	buf    uintptr
}

// i2cRdwr mirrors struct i2c_rdwr_ioctl_data from linux/i2c-dev.h
type i2cRdwr struct {
	msgs  uintptr
	nmsgs uint32
}

// I2CDriver is the real hardware driver for the AmpliPi preamp board,
// communicating via Linux I2C ioctl using I2C_RDWR for all transactions.
type I2CDriver struct {
	mu      sync.Mutex
	fd      int   // single shared fd for /dev/i2c-1
	units   []int // detected unit indices
	limiter *rate.Limiter
}

// NewI2C creates a new real I2C hardware driver.
func NewI2C() *I2CDriver {
	return &I2CDriver{
		fd:      -1,
		limiter: rate.NewLimiter(rate.Limit(maxOpsPerSec), 10),
	}
}

func (d *I2CDriver) Init(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Assign I2C address to the main preamp via UART.
	// The STM32 firmware starts with no I2C address and waits for this.
	// Protocol: send {0x41, 0x10, 0x0A} at 9600 baud on /dev/serial0.
	//   0x41 = 'A' (header), 0x10 = address (8-bit), 0x0A = '\n' (terminator).
	// The STM32 then initialises its I2C slave and forwards address+0x10 to
	// the next expander unit in the daisy-chain.
	if err := d.assignAddress(); err != nil {
		slog.Warn("i2c: UART address assignment failed (preamp may already be addressed)", "err", err)
	}
	// Wait for the address to propagate through the expander chain (~5ms per unit).
	time.Sleep(100 * time.Millisecond)

	// Open a single shared fd for all I2C_RDWR transactions.
	fd, err := unix.Open(i2cDevPath, unix.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("i2c: open %s: %w", i2cDevPath, err)
	}
	d.fd = fd

	var detected []int
	for unit, addr := range devAddrs {
		// Probe: try to read version register using I2C_RDWR (SMBus read_byte_data).
		// Generates the REPEATED START the STM32 firmware requires.
		_, err := d.readByteData(fd, addr, RegVersionMaj)
		if err != nil {
			slog.Debug("i2c: no response at address", "addr", fmt.Sprintf("0x%02x", addr), "err", err)
			break // addresses are sequential — first gap means no more units
		}
		slog.Info("i2c: preamp detected", "unit", unit, "addr", fmt.Sprintf("0x%02x", addr))
		detected = append(detected, unit)
	}

	if len(detected) == 0 {
		unix.Close(fd)
		d.fd = -1
		return fmt.Errorf("i2c: no AmpliPi preamp units detected on %s", i2cDevPath)
	}
	d.units = detected

	// Enforce digital-only on expander units (they have no analog inputs).
	for _, unit := range detected {
		if unit > 0 {
			addr := devAddrs[unit]
			_ = d.writeByteData(fd, addr, RegSrcAD, 0x0F) // all 4 sources digital
		}
	}

	return nil
}

func (d *I2CDriver) Write(ctx context.Context, unit int, reg Register, val byte) error {
	if err := d.limiter.Wait(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd < 0 {
		return fmt.Errorf("i2c: driver not initialized")
	}
	if unit < 0 || unit >= len(devAddrs) {
		return fmt.Errorf("i2c: invalid unit %d", unit)
	}
	addr := devAddrs[unit]
	return d.writeByteData(d.fd, addr, reg, val)
}

func (d *I2CDriver) Read(ctx context.Context, unit int, reg Register) (byte, error) {
	if err := d.limiter.Wait(ctx); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd < 0 {
		return 0, fmt.Errorf("i2c: driver not initialized")
	}
	if unit < 0 || unit >= len(devAddrs) {
		return 0, fmt.Errorf("i2c: invalid unit %d", unit)
	}
	addr := devAddrs[unit]
	return d.readByteData(d.fd, addr, reg)
}

// readByteData performs a combined write+read with REPEATED START (SMBus read_byte_data).
// This matches what the STM32 firmware expects: START→addr|W→reg→RS→addr|R→data→NACK→STOP
func (d *I2CDriver) readByteData(fd int, addr uint16, reg Register) (byte, error) {
	wbuf := [1]byte{reg}
	rbuf := [1]byte{}

	// Two i2c_msg: [write reg addr] + [read 1 byte], combined with I2C_RDWR ioctl
	msgs := [2]i2cMsg{
		{addr: addr, flags: 0, length: 1, buf: uintptr(unsafe.Pointer(&wbuf[0]))},
		{addr: addr, flags: i2cMsgRD, length: 1, buf: uintptr(unsafe.Pointer(&rbuf[0]))},
	}
	rdwr := i2cRdwr{msgs: uintptr(unsafe.Pointer(&msgs[0])), nmsgs: 2}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), i2cRdwrIOCTL, uintptr(unsafe.Pointer(&rdwr))); errno != 0 {
		return 0, fmt.Errorf("i2c: I2C_RDWR read: %w", errno)
	}
	return rbuf[0], nil
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

// Close releases the I2C file descriptor.
func (d *I2CDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fd >= 0 {
		unix.Close(d.fd)
		d.fd = -1
	}
}

// writeByteData performs a combined write of [reg, val] using I2C_RDWR.
// This is equivalent to smbus2.write_byte_data(addr, reg, val).
func (d *I2CDriver) writeByteData(fd int, addr uint16, reg Register, val byte) error {
	wbuf := [2]byte{reg, val}
	msgs := [1]i2cMsg{
		{addr: addr, flags: 0, length: 2, buf: uintptr(unsafe.Pointer(&wbuf[0]))},
	}
	rdwr := i2cRdwr{msgs: uintptr(unsafe.Pointer(&msgs[0])), nmsgs: 1}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), i2cRdwrIOCTL, uintptr(unsafe.Pointer(&rdwr))); errno != 0 {
		return fmt.Errorf("i2c: I2C_RDWR write 0x%02x reg=0x%02x: %w", addr, reg, errno)
	}
	return nil
}

const uartDev = "/dev/serial0"

// assignAddress sends the I2C address assignment to the main preamp via UART.
// The STM32 firmware starts with i2c_addr=0 (slave not initialised) and blocks
// until it receives this three-byte sequence.
func (d *I2CDriver) assignAddress() error {
	port, err := serial.Open(uartDev, &serial.Mode{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("open %s: %w", uartDev, err)
	}
	defer port.Close()

	// {0x41='A', 0x10=address, 0x0A='\n'}
	// The STM32 parses this as: header + i2c_addr + newline.
	_, err = port.Write([]byte{0x41, 0x10, 0x0A})
	if err != nil {
		return fmt.Errorf("write UART: %w", err)
	}
	slog.Debug("i2c: sent address assignment via UART", "addr", "0x10", "device", uartDev)
	return nil
}
