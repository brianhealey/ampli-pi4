//go:build linux

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

// TFT holds the ILI9341 display state.
type TFT struct {
	spiDev   spi.Conn
	dc       gpio.PinOut
	backlight gpio.PinOut
	width    int
	height   int
	img      *image.RGBA
}

const (
	// ILI9341 commands
	cmdSWRESET   = 0x01
	cmdSLPOUT    = 0x11
	cmdDISPON    = 0x29
	cmdCASet     = 0x2A
	cmdPASet     = 0x2B
	cmdRAMWR     = 0x2C
	cmdMADCTL    = 0x36
	cmdPIXFMT    = 0x3A

	// Display size
	displayWidth  = 320
	displayHeight = 240
)

// NewTFT initializes the TFT display.
func NewTFT() (*TFT, error) {
	// Initialize periph.io
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("periph.io init: %w", err)
	}

	// Open SPI device (SPI1 on CM4S carrier board)
	// The TFT is on SPI1, CS0 which is /dev/spidev1.0
	port, err := spireg.Open("/dev/spidev1.0")
	if err != nil {
		return nil, fmt.Errorf("open SPI: %w", err)
	}

	// Connect to SPI device with proper speed
	conn, err := port.Connect(16*physic.MegaHertz, spi.Mode0, 8)
	if err != nil {
		return nil, fmt.Errorf("connect SPI: %w", err)
	}

	// Get GPIO pins
	// DC (Data/Command) pin - GPIO39
	dc := gpioreg.ByName("GPIO39")
	if dc == nil {
		return nil, fmt.Errorf("failed to open GPIO39 (DC pin)")
	}

	// Backlight pin - GPIO12
	backlight := gpioreg.ByName("GPIO12")
	if backlight == nil {
		return nil, fmt.Errorf("failed to open GPIO12 (backlight pin)")
	}

	tft := &TFT{
		spiDev:    conn,
		dc:        dc,
		backlight: backlight,
		width:     displayWidth,
		height:    displayHeight,
		img:       image.NewRGBA(image.Rect(0, 0, displayWidth, displayHeight)),
	}

	// Initialize display
	if err := tft.init(); err != nil {
		return nil, fmt.Errorf("init display: %w", err)
	}

	slog.Info("TFT display initialized", "width", displayWidth, "height", displayHeight)
	return tft, nil
}

// init initializes the ILI9341 display controller.
// Initialization sequence matches Adafruit_CircuitPython_RGB_Display
func (t *TFT) init() error {
	// Turn on backlight
	if err := t.backlight.Out(gpio.High); err != nil {
		return fmt.Errorf("set backlight: %w", err)
	}

	// Software reset
	if err := t.writeCommand(cmdSWRESET); err != nil {
		return err
	}

	// Sleep out
	if err := t.writeCommand(cmdSLPOUT); err != nil {
		return err
	}

	// Power control
	if err := t.writeCommand(0xC0, 0x23); err != nil {
		return err
	}
	if err := t.writeCommand(0xC1, 0x10); err != nil {
		return err
	}

	// VCM control
	if err := t.writeCommand(0xC5, 0x3E, 0x28); err != nil {
		return err
	}
	if err := t.writeCommand(0xC7, 0x86); err != nil {
		return err
	}

	// Memory access control (MADCTL): 0x48 for default orientation
	// This matches Python adafruit library
	if err := t.writeCommand(cmdMADCTL, 0x48); err != nil {
		return err
	}

	// Pixel format: 16-bit color (RGB565)
	if err := t.writeCommand(cmdPIXFMT, 0x55); err != nil {
		return err
	}

	// Frame rate control
	if err := t.writeCommand(0xB1, 0x00, 0x18); err != nil {
		return err
	}

	// Display function control
	if err := t.writeCommand(0xB6, 0x08, 0x82, 0x27); err != nil {
		return err
	}

	// Gamma curves (simplified - not using full tables)
	if err := t.writeCommand(0xF2, 0x00); err != nil {
		return err
	}
	if err := t.writeCommand(0x26, 0x01); err != nil {
		return err
	}

	// Display on
	if err := t.writeCommand(cmdDISPON); err != nil {
		return err
	}

	slog.Debug("ILI9341 initialization complete")
	return nil
}

// writeCommand writes a command and optional data bytes to the display.
func (t *TFT) writeCommand(cmd byte, data ...byte) error {
	// DC low = command
	if err := t.dc.Out(gpio.Low); err != nil {
		return err
	}

	// Write command byte
	if err := t.spiDev.Tx([]byte{cmd}, nil); err != nil {
		return err
	}

	// If there's data, write it with DC high
	if len(data) > 0 {
		if err := t.dc.Out(gpio.High); err != nil {
			return err
		}
		if err := t.spiDev.Tx(data, nil); err != nil {
			return err
		}
	}

	return nil
}

// setWindow sets the drawing window on the display.
func (t *TFT) setWindow(x0, y0, x1, y1 int) error {
	// Column address set
	if err := t.writeCommand(cmdCASet,
		byte(x0>>8), byte(x0),
		byte(x1>>8), byte(x1)); err != nil {
		return err
	}

	// Page address set
	if err := t.writeCommand(cmdPASet,
		byte(y0>>8), byte(y0),
		byte(y1>>8), byte(y1)); err != nil {
		return err
	}

	return nil
}

// Display renders the internal image buffer to the screen.
func (t *TFT) Display() error {
	// Set full screen window
	if err := t.setWindow(0, 0, t.width-1, t.height-1); err != nil {
		return err
	}

	// Prepare RAM write
	if err := t.dc.Out(gpio.Low); err != nil {
		return err
	}
	if err := t.spiDev.Tx([]byte{cmdRAMWR}, nil); err != nil {
		return err
	}

	// DC high for data
	if err := t.dc.Out(gpio.High); err != nil {
		return err
	}

	// Convert RGBA to RGB565 and write in chunks
	// SPI driver has a max transfer size of 4096 bytes
	const chunkSize = 4096
	totalBytes := t.width * t.height * 2 // 2 bytes per pixel (RGB565)
	buf := make([]byte, chunkSize)

	pixelIdx := 0
	for offset := 0; offset < totalBytes; offset += chunkSize {
		// Calculate how many bytes to send in this chunk
		remaining := totalBytes - offset
		size := chunkSize
		if remaining < chunkSize {
			size = remaining
		}

		// Fill buffer with RGB565 pixels
		for i := 0; i < size; i += 2 {
			x := pixelIdx % t.width
			y := pixelIdx / t.width
			r, g, b, _ := t.img.At(x, y).RGBA()

			// Convert from 16-bit RGBA to 8-bit RGB
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)

			// Convert to RGB565 format (5 bits red, 6 bits green, 5 bits blue)
			rgb565 := uint16((r8&0xF8)<<8) | uint16((g8&0xFC)<<3) | uint16(b8>>3)

			// Big-endian (MSB first) - matches Python ">H" format
			buf[i] = byte(rgb565 >> 8)
			buf[i+1] = byte(rgb565)
			pixelIdx++
		}

		// Write chunk
		if err := t.spiDev.Tx(buf[:size], nil); err != nil {
			return err
		}
	}

	return nil
}

// Clear clears the screen to the specified color.
func (t *TFT) Clear(c color.Color) {
	draw.Draw(t.img, t.img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
}

// DrawText draws text at the specified position.
func (t *TFT) DrawText(x, y int, text string, col color.Color) {
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}

	d := &font.Drawer{
		Dst:  t.img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(text)
}

// RenderStatus renders the status display matching the Python TFT layout.
func (t *TFT) RenderStatus(status *Status) error {
	slog.Debug("Rendering TFT display", "zones", len(status.Zones), "sources", len(status.Sources))

	// TEST: Try different MADCTL values for rotation=270
	// Python uses rotation=270, which could be:
	// 0x20 (MV), 0xE8 (MY|MX|MV|BGR), 0xA8, etc.
	// Let's try 0xE8 first
	if err := t.writeCommand(cmdMADCTL, 0xE8); err != nil {
		return err
	}
	slog.Info("TFT: Set MADCTL to 0xE8 for rotation=270")

	// TEST: Fill with simple pattern: red on left half, blue on right half
	for y := 0; y < t.height; y++ {
		for x := 0; x < t.width; x++ {
			if x < t.width/2 {
				t.img.Set(x, y, color.RGBA{255, 0, 0, 255}) // Red left
			} else {
				t.img.Set(x, y, color.RGBA{0, 0, 255, 255}) // Blue right
			}
		}
	}

	// Display the buffer and return early for testing
	if err := t.Display(); err != nil {
		return err
	}
	slog.Info("TFT test pattern: red left, blue right")
	return nil

	// TODO: Remove test pattern code above and uncomment below when working
	/*
	// Clear to black
	t.Clear(color.Black)

	// Define colors
	white := color.RGBA{255, 255, 255, 255}
	yellow := color.RGBA{255, 255, 0, 255}
	green := color.RGBA{0, 255, 0, 255}
	lightGray := color.RGBA{153, 153, 153, 255}

	// Character dimensions (7x13 font)
	const cw = 7
	const ch = 13

	// Line 1: Disk usage
	diskColor := gradientColor(status.DiskPercent)
	t.DrawText(1*cw, 1*ch+2, "Disk:", white)
	t.DrawText(7*cw, 1*ch+2, fmt.Sprintf("%.1f%%", status.DiskPercent), diskColor)
	t.DrawText(14*cw, 1*ch+2, fmt.Sprintf("%.2f/%.2f GB", status.DiskUsedGB, status.DiskTotalGB), diskColor)

	// Line 2: IP address
	ipStr := fmt.Sprintf("%s, %s.local", status.IP, status.Hostname)
	t.DrawText(1*cw, 2*ch+2, fmt.Sprintf("IP:   %s", ipStr), white)

	// Line 3: Password
	passColor := yellow // Default password = yellow
	t.DrawText(1*cw, 3*ch+2, "Password: ", white)
	t.DrawText(11*cw, 3*ch+2, status.Password, passColor)

	// Line 0 (status): Zone/source emoji status
	playing := 0
	muted := 0
	for _, z := range status.Zones {
		if !z.Mute {
			playing++
		} else {
			muted++
		}
	}
	statusStr := fmt.Sprintf("Status: ▶x%d ⏸x%d", playing, muted)
	t.DrawText(1*cw, 0*ch+2, statusStr, white)

	// Expander count (if > 0)
	if status.Expanders > 0 {
		t.DrawText(22*cw, 0*ch+2, fmt.Sprintf("Expanders: %d", status.Expanders), white)
	}

	// Source labels and playing indicators
	ys := 4*ch + ch/2

	// Draw top separator line
	t.DrawHLine(cw, t.width-2*cw, ys-3, 2, lightGray)

	// Source 1-4 labels and playing indicators
	sources := []string{"Source 1:", "Source 2:", "Source 3:", "Source 4:"}
	for i := 0; i < 4 && i < len(sources); i++ {
		t.DrawText(1*cw, int(float64(ys)+float64(i)*1.1*float64(ch)), sources[i], white)

		// Draw source name and playing indicator if available
		if i < len(status.Sources) {
			src := status.Sources[i]
			// Playing indicator (green triangle)
			if src.Playing {
				xp := 10*cw - cw/2
				yp := ys + i*ch + 3
				t.DrawTriangle(xp, yp, cw-3, ch, green)
			}
			// Source name
			if src.Name != "" {
				t.DrawText(11*cw, ys+i*ch, src.Name, yellow)
			}
		}
	}

	// Draw bottom separator line
	t.DrawHLine(cw, t.width-2*cw, ys+4*ch+2, 2, lightGray)

	// Volume bars for zones (below source section)
	t.DrawVolumeBars(status.Zones, cw, 9*ch-2, t.width-2*cw, t.height-9*ch)

	// Display the buffer
	if err := t.Display(); err != nil {
		return err
	}

	slog.Debug("TFT display render complete")
	return nil
	*/
}

// gradientColor returns a color based on percentage (green->yellow->red).
func gradientColor(percent float64) color.Color {
	if percent < 50 {
		return color.RGBA{0, 255, 0, 255} // Green
	} else if percent < 75 {
		return color.RGBA{255, 255, 0, 255} // Yellow
	}
	return color.RGBA{255, 0, 0, 255} // Red
}

// DrawHLine draws a horizontal line.
func (t *TFT) DrawHLine(x0, x1, y, width int, col color.Color) {
	for i := 0; i < width; i++ {
		for x := x0; x <= x1; x++ {
			t.img.Set(x, y+i, col)
		}
	}
}

// DrawTriangle draws a filled triangle (for playing indicator).
func (t *TFT) DrawTriangle(x, y, w, h int, col color.Color) {
	// Simple right-pointing triangle
	for dy := 0; dy < h; dy++ {
		// Calculate width at this row
		dx := (dy * w) / h
		if dy >= h/2 {
			dx = ((h - dy) * w) / h
		}
		for i := 0; i < dx; i++ {
			t.img.Set(x+i, y+dy, col)
		}
	}
}

// DrawVolumeBars draws volume bars for zones.
func (t *TFT) DrawVolumeBars(zones []ZoneInfo, x, y, width, height int) {
	if len(zones) == 0 {
		return
	}

	// Calculate bar dimensions
	barWidth := width / len(zones)
	if barWidth > 40 {
		barWidth = 40
	}
	barSpacing := (width - barWidth*len(zones)) / (len(zones) + 1)

	white := color.RGBA{255, 255, 255, 255}
	green := color.RGBA{0, 255, 0, 255}
	gray := color.RGBA{64, 64, 64, 255}

	for i, zone := range zones {
		// Calculate bar position
		barX := x + barSpacing*(i+1) + barWidth*i
		barHeight := height - 20 // Leave room for label

		// Draw zone number label
		label := fmt.Sprintf("Z%d", zone.ID+1)
		t.DrawText(barX, y+barHeight+10, label, white)

		// Calculate fill height based on volume
		// Volume range: -79 to 0 dB
		volumePercent := float64(zone.Volume+79) / 79.0
		fillHeight := int(volumePercent * float64(barHeight))

		// Draw bar outline
		for py := y; py < y+barHeight; py++ {
			t.img.Set(barX, py, white)
			t.img.Set(barX+barWidth-1, py, white)
		}
		for px := barX; px < barX+barWidth; px++ {
			t.img.Set(px, y, white)
			t.img.Set(px, y+barHeight-1, white)
		}

		// Fill bar based on volume
		fillColor := green
		if zone.Mute {
			fillColor = gray
		}

		for py := 0; py < fillHeight; py++ {
			for px := 1; px < barWidth-1; px++ {
				t.img.Set(barX+px, y+barHeight-1-py, fillColor)
			}
		}
	}
}
