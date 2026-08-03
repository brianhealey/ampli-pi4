package main

import "testing"

// The ILI9341 takes RGB565: 5 bits red (15..11), 6 green (10..5), 5 blue (4..0).
// Getting the Go shift widths wrong silently drops whole channels — red renders
// black while blue still looks right, which is easy to mistake for a wiring or
// colour-order problem rather than an arithmetic one.
func TestRGB565PacksEachChannel(t *testing.T) {
	tests := []struct {
		name       string
		r, g, b    uint8
		want       uint16
	}{
		{"black", 0x00, 0x00, 0x00, 0x0000},
		{"white", 0xFF, 0xFF, 0xFF, 0xFFFF},
		{"red", 0xFF, 0x00, 0x00, 0xF800},
		{"green", 0x00, 0xFF, 0x00, 0x07E0},
		{"blue", 0x00, 0x00, 0xFF, 0x001F},
		{"mid grey", 0x80, 0x80, 0x80, 0x8410},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rgb565(tc.r, tc.g, tc.b)
			if got != tc.want {
				t.Errorf("rgb565(%#02x, %#02x, %#02x) = %#04x, want %#04x",
					tc.r, tc.g, tc.b, got, tc.want)
			}
		})
	}
}
