package main

// rgb565 packs 8-bit R, G and B into the 16-bit RGB565 word the ILI9341
// expects: 5 bits red (15..11), 6 bits green (10..5), 5 bits blue (4..0).
// Each channel is widened to uint16 *before* shifting. Shifting the uint8
// first discards the bits that are supposed to end up in the high byte, which
// zeroes red entirely and truncates green.
func rgb565(r8, g8, b8 uint8) uint16 {
	return uint16(r8&0xF8)<<8 | uint16(g8&0xFC)<<3 | uint16(b8>>3)
}
