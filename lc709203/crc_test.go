package lc709203

import "testing"

// TestCRC8DatasheetVectors pins the CRC-8 implementation against the two
// worked examples the datasheet itself gives in "I2C Communication
// Protocol": a read of cell voltage = 3778 mV, and a standalone write
// example. Both include the I2C address byte(s), which is the detail most
// likely to be missed.
func TestCRC8DatasheetVectors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want uint8
	}{
		{
			name: "read: write-addr,cmd,read-addr,data-low,data-high for cell voltage 3778 mV",
			data: []byte{0x16, 0x09, 0x17, 0xC2, 0x0E},
			want: 0x86,
		},
		{
			name: "write: write-addr,cmd,data-low,data-high",
			data: []byte{0x16, 0x09, 0x55, 0xAA},
			want: 0x3B,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := crc8(c.data); got != c.want {
				t.Errorf("crc8(% X) = %#02x, want %#02x", c.data, got, c.want)
			}
		})
	}
}

// TestCRC8CoversAddressByte checks that the address byte actually
// contributes to the result, not just the command and data - the specific
// mistake the datasheet warns is easy to make.
func TestCRC8CoversAddressByte(t *testing.T) {
	sameCmdAndData := crc8([]byte{0x16, 0x09, 0x17, 0xC2, 0x0E})
	differentAddress := crc8([]byte{0x20, 0x09, 0x21, 0xC2, 0x0E})
	if sameCmdAndData == differentAddress {
		t.Error("CRC did not change when only the address byte changed")
	}
}
