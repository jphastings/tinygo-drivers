// Package st25dv provides a driver for the STMicroelectronics ST25DV
// dynamic NFC/RFID tag: a dual-interface EEPROM that can be read and written
// both over I2C and over ISO 15693 RF (NFC Forum Type 5 Tag), for example by
// a smartphone.
//
// The ST25DV04K, ST25DV16K and ST25DV64K variants are supported, the user
// memory size is detected automatically. Tested with the Adafruit ST25DV16K
// breakout: https://www.adafruit.com/product/4701
//
// Datasheet:
//
//	https://www.st.com/resource/en/datasheet/st25dv16k.pdf
//
// This driver is inspired by the STMicroelectronics C driver:
//
//	https://github.com/STMicroelectronics/stm32-st25dv
package st25dv

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConnected  = errors.New("st25dv: no ST25DV found on the bus")
	errNotConfigured = errors.New("st25dv: call Configure first")
	errOutOfRange    = errors.New("st25dv: address out of range")
	errWriteTimeout  = errors.New("st25dv: timeout waiting for EEPROM write cycle")
)

// Device wraps an I2C connection to an ST25DV device
type Device struct {
	bus drivers.I2C

	// User memory size in bytes, detected during Configure
	memSize uint32

	// Scratch buffer: two register address bytes plus one write chunk
	buf [2 + writeChunkSize]byte
}

// New creates a new ST25DV connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus: bus,
	}
}

// Connected checks whether an ST25DV answers on the bus, by verifying the
// ISO 15693 and STMicroelectronics prefixes of its UID
func (d *Device) Connected() bool {
	uid, err := d.UID()
	return err == nil && uid[0] == uidISO15693Prefix && uid[1] == uidSTManufacturerID
}

// Configure checks the device is responding and detects its user memory
// size. The chip needs no further set up: it works out of the box on both
// its I2C and RF interfaces.
func (d *Device) Configure() error {
	if !d.Connected() {
		return errNotConnected
	}

	// MEM_SIZE holds the last block number (0-based) and BLK_SIZE the block
	// size in bytes, both minus one
	var raw [3]byte
	err := d.readRegister(AddressSystem, REG_MEM_SIZE_LSB, raw[:])
	if err != nil {
		return err
	}
	blocks := (uint32(raw[1])<<8 | uint32(raw[0])) + 1
	d.memSize = blocks * (uint32(raw[2]) + 1)

	return nil
}

// UID returns the ISO 15693 unique identifier of the chip, most significant
// byte (always 0xE0) first
func (d *Device) UID() (uid [8]byte, err error) {
	err = d.readRegister(AddressSystem, REG_UID, uid[:])
	if err != nil {
		return
	}

	// The UID is stored least significant byte first
	for i := 0; i < 4; i++ {
		uid[i], uid[7-i] = uid[7-i], uid[i]
	}
	return
}

// Size returns the user memory size in bytes: 512 on the ST25DV04K, 2048 on
// the ST25DV16K and 8192 on the ST25DV64K
func (d *Device) Size() int64 {
	return int64(d.memSize)
}

// ReadAt reads len(p) bytes of user memory starting at byte offset off. It
// implements io.ReaderAt.
func (d *Device) ReadAt(p []byte, off int64) (n int, err error) {
	err = d.checkBounds(len(p), off)
	if err != nil {
		return 0, err
	}

	err = d.readRegister(AddressUser, uint16(off), p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteAt writes len(p) bytes of user memory starting at byte offset off. It
// implements io.WriterAt.
//
// The memory is written in chunks, waiting for the chip's internal write
// cycle after each: worst case a little over 5ms per 4 bytes written.
func (d *Device) WriteAt(p []byte, off int64) (n int, err error) {
	err = d.checkBounds(len(p), off)
	if err != nil {
		return 0, err
	}

	for n < len(p) {
		chunk := len(p) - n
		if chunk > writeChunkSize {
			chunk = writeChunkSize
		}

		address := uint16(off) + uint16(n)
		d.buf[0] = uint8(address >> 8)
		d.buf[1] = uint8(address)
		copy(d.buf[2:], p[n:n+chunk])

		err = d.bus.Tx(uint16(AddressUser), d.buf[:2+chunk], nil)
		if err != nil {
			return n, err
		}

		err = d.waitWriteCycle()
		if err != nil {
			return n, err
		}
		n += chunk
	}

	return n, nil
}

// readRegister reads from a 16-bit register address on one of the two I2C
// device addresses
func (d *Device) readRegister(devAddr uint8, register uint16, data []byte) error {
	d.buf[0] = uint8(register >> 8)
	d.buf[1] = uint8(register)
	return d.bus.Tx(uint16(devAddr), d.buf[:2], data)
}

// waitWriteCycle waits for an EEPROM write cycle to finish. The chip does
// not acknowledge its I2C address while writing (up to 5ms per 4-byte row),
// so poll with a harmless read until it answers again.
func (d *Device) waitWriteCycle() error {
	// Generous upper bound for a full chunk plus row misalignment
	const timeout = ((writeChunkSize/4 + 2) * 5) * 2 * time.Millisecond

	for start := time.Now(); time.Since(start) < timeout; {
		if err := d.bus.Tx(uint16(AddressUser), nil, d.buf[:1]); err == nil {
			return nil
		}
		time.Sleep(500 * time.Microsecond)
	}
	return errWriteTimeout
}

func (d *Device) checkBounds(length int, off int64) error {
	if d.memSize == 0 {
		return errNotConfigured
	}
	if off < 0 || off+int64(length) > int64(d.memSize) {
		return errOutOfRange
	}
	return nil
}
