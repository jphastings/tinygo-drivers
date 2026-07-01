// Package trustm provides a driver for the Infineon OPTIGA Trust M
// (SLS32AIA) security chip: a hardware trust anchor with a true random
// number generator, protected key and data storage, and cryptographic
// coprocessor. Tested with the Adafruit breakout:
// https://www.adafruit.com/product/4351
//
// Only a subset of the chip's functionality is implemented: the Infineon
// I2C protocol transport, opening the application, reading true random
// bytes, and reading data objects such as the coprocessor UID. The shielded
// (encrypted) connection and the cryptographic operations (ECDSA, ECDH,
// RSA, AES, key management) are not implemented. Command and response
// packets are limited to a single protocol frame, which is sufficient for
// the commands offered here.
//
// Datasheet and protocol references:
//
//	https://github.com/Infineon/optiga-trust-m/blob/develop/documents/OPTIGA_Trust_M_Datasheet_v3.61.pdf
//	https://github.com/Infineon/optiga-trust-m/blob/develop/documents/Infineon_I2C_Protocol_v2.03.pdf
//
// This driver is a synchronous re-implementation of the Infineon host
// library protocol stack:
//
//	https://github.com/Infineon/optiga-trust-m
package trustm

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

var (
	errNotConfigured  = errors.New("trustm: call Configure first")
	errTimeout        = errors.New("trustm: timeout waiting for response")
	errBadFrame       = errors.New("trustm: malformed frame received")
	errCRCMismatch    = errors.New("trustm: frame CRC mismatch")
	errNack           = errors.New("trustm: frame rejected by chip")
	errUnexpectedAck  = errors.New("trustm: unexpected acknowledgement")
	errChained        = errors.New("trustm: chained response not supported")
	errTooLong        = errors.New("trustm: request too long for one frame")
	errLengthOutRange = errors.New("trustm: length out of range")
	errDeviceError    = errors.New("trustm: command failed on the chip")
	errShortResponse  = errors.New("trustm: response shorter than expected")
)

// The largest frame the chip can be asked to support (IFX_I2C_FRAME_SIZE in
// the Infineon host library); the chip default is 0x110 bytes
const maxFrameSize = 277

// Byte layout of the transmit buffer: the DATA register address, then the
// data link frame (FCTR, length), then the transport layer packet control
// byte, then the command APDU. The frame CRC follows the APDU.
const (
	fctrOffset = 1
	pctrOffset = 4
	apduOffset = 5
)

// A command APDU is [command, param, 16-bit InData length, InData...]; a
// response APDU is [status, undefined, 16-bit OutData length, OutData...]
const apduHeaderSize = 4

const (
	// Pause after each bus transaction so the chip can process it
	// (PL_GUARD_TIME_INTERVAL_US in the Infineon host library)
	guardTime = 50 * time.Microsecond

	// A sleeping chip NACKs its address while waking up: retry interval
	// and count (PL_POLLING_INVERVAL_US, PL_POLLING_MAX_CNT)
	wakeInterval = time.Millisecond
	wakeRetries  = 200

	// Response polling interval (PL_DATA_POLLING_INVERVAL_US) and
	// count. The host library allows several minutes; five seconds is
	// plenty for the commands implemented here.
	pollInterval = 5 * time.Millisecond
	pollRetries  = 1000

	// Boot time after a soft reset: the datasheet guarantees start-up
	// within 15ms
	resetStartup = 15 * time.Millisecond
)

// Device wraps an I2C connection to an OPTIGA Trust M
type Device struct {
	bus drivers.I2C

	// I2C device address, Address unless reconfigured
	Address uint16

	// Frame size read back from DATA_REG_LEN during Configure
	frameSize uint16

	// Data link layer frame sequence counters: last sent, last received
	txSeq uint8
	rxSeq uint8

	txBuf  [1 + maxFrameSize]byte
	rxBuf  [maxFrameSize]byte
	ackBuf [1 + 5]byte
	state  [4]byte
}

// New creates a new OPTIGA Trust M connection. The I2C bus must already be
// configured.
//
// This function only creates the Device object, it does not touch the
// device.
func New(bus drivers.I2C) Device {
	return Device{
		bus:     bus,
		Address: Address,
		txSeq:   seqInit,
		rxSeq:   seqInit,
	}
}

// Connected checks whether an OPTIGA Trust M answers on the bus
func (d *Device) Connected() bool {
	return d.readRegister(REG_I2C_STATE, d.state[:]) == nil
}

// Configure soft-resets the chip, reads the frame size it supports and
// opens the OPTIGA application, after which commands can be issued
func (d *Device) Configure() error {
	d.txSeq, d.rxSeq = seqInit, seqInit

	err := d.readRegister(REG_I2C_STATE, d.state[:])
	if err != nil {
		return err
	}

	// Soft reset for a known-clean state; skipped on chips without the
	// SOFT_RESET register
	if d.state[0]&stateSoftReset != 0 {
		d.txBuf[0] = REG_SOFT_RESET
		d.txBuf[1] = 0x00
		d.txBuf[2] = 0x00
		err = d.tx(d.txBuf[:3], nil)
		if err != nil {
			return err
		}
		time.Sleep(resetStartup)
	}

	var size [2]byte
	err = d.readRegister(REG_DATA_REG_LEN, size[:])
	if err != nil {
		return err
	}
	d.frameSize = uint16(size[0])<<8 | uint16(size[1])
	if d.frameSize < dlOverhead+1 {
		d.frameSize = 0
		return errBadFrame
	}
	if d.frameSize > maxFrameSize {
		d.frameSize = maxFrameSize
	}

	copy(d.txBuf[apduOffset+apduHeaderSize:], applicationID[:])
	_, err = d.command(CMD_OPEN_APPLICATION, paramInitApp, len(applicationID))
	if err != nil {
		d.frameSize = 0
		return err
	}
	return nil
}

// GetRandom fills rnd with up to 256 true random bytes from the chip's
// AIS-31 compliant random number generator
func (d *Device) GetRandom(rnd []byte) error {
	if len(rnd) == 0 {
		return nil
	}
	if len(rnd) > maxRandomLength {
		return errLengthOutRange
	}

	// The chip refuses requests below 8 bytes; extra bytes are discarded
	request := len(rnd)
	if request < minRandomLength {
		request = minRandomLength
	}

	d.txBuf[apduOffset+apduHeaderSize] = byte(request >> 8)
	d.txBuf[apduOffset+apduHeaderSize+1] = byte(request)
	out, err := d.command(CMD_GET_RANDOM, paramTRNG, 2)
	if err != nil {
		return err
	}
	if len(out) < len(rnd) {
		return errShortResponse
	}
	copy(rnd, out)
	return nil
}

// GetDataObject reads a data object, e.g. a certificate or the coprocessor
// UID, starting at the given byte offset within the object. It reads at
// most len(data) bytes and returns the number of bytes the chip returned,
// which is smaller when the object ends before offset+len(data).
func (d *Device) GetDataObject(oid uint16, offset uint16, data []byte) (int, error) {
	if d.frameSize == 0 {
		return 0, errNotConfigured
	}
	// Responses are limited to a single frame in this driver
	maxRead := int(d.frameSize) - dlOverhead - 1 - apduHeaderSize
	if len(data) > maxRead {
		return 0, errTooLong
	}

	in := d.txBuf[apduOffset+apduHeaderSize:]
	in[0] = byte(oid >> 8)
	in[1] = byte(oid)
	in[2] = byte(offset >> 8)
	in[3] = byte(offset)
	in[4] = byte(len(data) >> 8)
	in[5] = byte(len(data))
	out, err := d.command(CMD_GET_DATA_OBJECT, paramReadData, 6)
	if err != nil {
		return 0, err
	}
	return copy(data, out), nil
}

// UID returns the 27-byte coprocessor UID, which uniquely identifies the
// chip: it includes the chip type, batch number, wafer position and
// firmware version
func (d *Device) UID() (uid [UIDLength]byte, err error) {
	n, err := d.GetDataObject(OID_COPROCESSOR_UID, 0, uid[:])
	if err == nil && n != len(uid) {
		err = errShortResponse
	}
	return
}

// command sends the APDU whose InData was already placed in
// d.txBuf[apduOffset+apduHeaderSize:] and returns the OutData of the
// response APDU, valid until the next operation
func (d *Device) command(cmd, param uint8, inLen int) ([]byte, error) {
	apdu := d.txBuf[apduOffset:]
	apdu[0] = cmd
	apdu[1] = param
	apdu[2] = byte(inLen >> 8)
	apdu[3] = byte(inLen)

	resp, err := d.transceive(apduHeaderSize + inLen)
	if err != nil {
		return nil, err
	}
	if len(resp) < apduHeaderSize {
		return nil, errShortResponse
	}
	if resp[0] != 0x00 {
		return nil, errDeviceError
	}
	outLen := int(resp[2])<<8 | int(resp[3])
	if apduHeaderSize+outLen > len(resp) {
		return nil, errShortResponse
	}
	return resp[apduHeaderSize : apduHeaderSize+outLen], nil
}

// Frame overhead of the data link layer: FCTR, 2 length bytes, 2 CRC bytes
const dlOverhead = 5

// transceive wraps the APDU already placed in d.txBuf[apduOffset:] in a
// transport packet and data link frame, sends it, and waits for the
// acknowledgement and response frames. It returns the response APDU.
func (d *Device) transceive(apduLen int) ([]byte, error) {
	if d.frameSize == 0 {
		return nil, errNotConfigured
	}
	payloadLen := uint16(apduLen) + 1 // transport layer PCTR byte
	if int(payloadLen)+dlOverhead > int(d.frameSize) {
		return nil, errTooLong
	}

	d.txSeq = (d.txSeq + 1) & fctrAckNrMask
	d.txBuf[0] = REG_DATA
	d.txBuf[fctrOffset] = d.txSeq<<fctrFrameNrPos | d.rxSeq // seqctr: ACK
	d.txBuf[fctrOffset+1] = byte(payloadLen >> 8)
	d.txBuf[fctrOffset+2] = byte(payloadLen)
	d.txBuf[pctrOffset] = pctrChainNone
	crc := crc16(d.txBuf[fctrOffset : pctrOffset+payloadLen])
	d.txBuf[pctrOffset+payloadLen] = byte(crc >> 8)
	d.txBuf[pctrOffset+payloadLen+1] = byte(crc)

	err := d.tx(d.txBuf[:1+dlOverhead+int(payloadLen)], nil)
	if err != nil {
		return nil, err
	}

	// The chip acknowledges our frame either with a control frame, or
	// piggybacked on its response data frame; wait for the data frame
	for attempt := 0; attempt < 4; attempt++ {
		n, err := d.readFrame()
		if err != nil {
			return nil, err
		}
		fctr := d.rxBuf[0]
		if fctr&fctrAckNrMask != d.txSeq {
			return nil, errUnexpectedAck
		}
		seqctr := (fctr & fctrSeqctrMask) >> fctrSeqctrPos
		if seqctr == seqctrNack {
			return nil, errNack
		}
		if seqctr != seqctrAck {
			return nil, errBadFrame
		}

		if fctr&fctrControlFrame != 0 {
			continue // our frame was accepted, await the data frame
		}
		if (fctr&fctrFrameNrMask)>>fctrFrameNrPos != (d.rxSeq+1)&fctrAckNrMask {
			return nil, errBadFrame
		}
		d.rxSeq = (d.rxSeq + 1) & fctrAckNrMask
		err = d.sendAck()
		if err != nil {
			return nil, err
		}
		if d.rxBuf[3] != pctrChainNone {
			return nil, errChained
		}
		return d.rxBuf[4 : n-2], nil
	}
	return nil, errTimeout
}

// sendAck sends a control frame acknowledging the last received data frame
func (d *Device) sendAck() error {
	d.ackBuf[0] = REG_DATA
	d.ackBuf[1] = fctrControlFrame | seqctrAck<<fctrSeqctrPos | d.rxSeq
	d.ackBuf[2] = 0x00
	d.ackBuf[3] = 0x00
	crc := crc16(d.ackBuf[1:4])
	d.ackBuf[4] = byte(crc >> 8)
	d.ackBuf[5] = byte(crc)
	return d.tx(d.ackBuf[:], nil)
}

// readFrame polls I2C_STATE until the chip announces a frame, reads it from
// the DATA register into d.rxBuf, and verifies its length and CRC
func (d *Device) readFrame() (int, error) {
	var frameLen uint16
	for attempt := 0; ; attempt++ {
		err := d.readRegister(REG_I2C_STATE, d.state[:])
		if err != nil {
			return 0, err
		}
		// The pending frame length is in bytes 2 and 3 of I2C_STATE
		frameLen = uint16(d.state[2])<<8 | uint16(d.state[3])
		if d.state[0]&stateResponseReady != 0 &&
			frameLen >= dlOverhead && frameLen <= d.frameSize {
			break
		}
		if attempt >= pollRetries {
			return 0, errTimeout
		}
		time.Sleep(pollInterval)
	}

	err := d.readRegister(REG_DATA, d.rxBuf[:frameLen])
	if err != nil {
		return 0, err
	}
	payloadLen := uint16(d.rxBuf[1])<<8 | uint16(d.rxBuf[2])
	if int(frameLen) != dlOverhead+int(payloadLen) {
		return 0, errBadFrame
	}
	crc := uint16(d.rxBuf[frameLen-2])<<8 | uint16(d.rxBuf[frameLen-1])
	if crc16(d.rxBuf[:frameLen-2]) != crc {
		return 0, errCRCMismatch
	}
	return int(frameLen), nil
}

// readRegister selects a register and reads its contents. The chip needs a
// stop condition between the two, so they are separate transactions.
func (d *Device) readRegister(reg uint8, buf []byte) error {
	d.txBuf[0] = reg
	err := d.tx(d.txBuf[:1], nil)
	if err != nil {
		return err
	}
	return d.tx(nil, buf)
}

// tx performs one I2C transaction, retrying while a sleeping chip NACKs its
// address, and observes the guard time the chip needs afterwards
func (d *Device) tx(w, r []byte) error {
	var err error
	for attempt := 0; attempt < wakeRetries; attempt++ {
		err = d.bus.Tx(d.Address, w, r)
		if err == nil {
			break
		}
		time.Sleep(wakeInterval)
	}
	if err != nil {
		return err
	}
	time.Sleep(guardTime)
	return nil
}

// crc16 computes the frame check sequence of the Infineon I2C protocol:
// CRC-16/KERMIT (reflected polynomial 0x1021, zero initial value). This is
// a transcription of ifx_i2c_dl_calc_crc_byte in the Infineon host library.
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		h1 := (crc ^ uint16(b)) & 0xFF
		h2 := h1 & 0x0F
		h3 := (h2 << 4) ^ h1
		h4 := h3 >> 4
		crc = (((h3<<1)^h4)<<4^h2)<<3 ^ h4 ^ (crc >> 8)
	}
	return crc
}
