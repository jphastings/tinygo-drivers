package lc709203

// CRC-8 packet error checking.
//
// Every read and write is followed by an extra CRC-8 byte covering the
// whole transaction - including the I2C address byte(s), not just the
// command and data. The datasheet calls this "CRC-8-ATM": polynomial
// x^8+x^2+x+1 (0x07), initial value 0x00, MSB-first, no input/output
// reflection and no final XOR. That last point is worth being explicit
// about: the more common "CRC-8/ATM (HEC)" parametrisation XORs the result
// with 0x55, but the LC709203F's own worked examples do not, so this
// implementation follows the datasheet's numbers rather than the name.
//
// On a read, the covered bytes are: the write-phase address byte (device
// address<<1, R/W=0), the command code, the read-phase address byte
// (device address<<1, R/W=1), and the two data bytes (low then high) the
// device just returned. Datasheet example, reading cell voltage = 3778 mV:
//
//	0x16, 0x09, 0x17, 0xC2, 0x0E -> 0x86
//
// On a write, the covered bytes are: the write-phase address byte, the
// command code, and the two data bytes (low then high) being written.
// Datasheet example:
//
//	0x16, 0x09, 0x55, 0xAA -> 0x3B
//
// Both examples are pinned as test vectors in crc_test.go.
//
// Getting this wrong has no effect at the I2C protocol level - the ACK
// still comes back - so a broken CRC only shows up as data that is wrong
// or (per the datasheet) a write that never actually took effect. See
// io.go for where these byte sequences are built.
const crc8Poly = 0x07

func crc8(data []byte) uint8 {
	var crc uint8
	for _, b := range data {
		crc ^= b
		for i := 0; i < 8; i++ {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ crc8Poly
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
