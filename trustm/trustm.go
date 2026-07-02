// Package trustm provides a driver for the Infineon OPTIGA Trust M
// (SLS32AIA) security chip: a hardware trust anchor with a true random
// number generator, protected key and data storage, and cryptographic
// coprocessor. Tested with the Adafruit breakout:
// https://www.adafruit.com/product/4351
//
// Only a subset of the chip's functionality is implemented: the Infineon
// I2C protocol transport including packet chaining, opening the
// application, true random bytes, reading and writing data objects,
// SHA-256 hashing, ECDSA signing and verification, ECC key pair
// generation (NIST P-256 and P-384) and ECDH shared secrets. The shielded
// (encrypted) connection, RSA, AES/HMAC and protected key export are not
// implemented. Beware that without the shielded connection everything on
// the I2C bus - commands, data objects, even ECDH shared secrets - is
// unencrypted and visible to anyone able to probe the bus.
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
	errBrokenChain    = errors.New("trustm: broken packet chain")
	errTooLong        = errors.New("trustm: packet too long for the driver's buffers")
	errLengthOutRange = errors.New("trustm: length out of range")
	errDeviceError    = errors.New("trustm: command failed on the chip")
	errShortResponse  = errors.New("trustm: response shorter than expected")
	errShortBuffer    = errors.New("trustm: response larger than the buffer")
)

// The largest frame the chip can be asked to support (IFX_I2C_FRAME_SIZE in
// the Infineon host library); the chip default is 0x110 bytes
const maxFrameSize = 277

// The largest command or response packet (APDU) the driver can exchange;
// packets bigger than one frame are chained across several frames
const maxPacketSize = 700

// Byte layout of the frame transmit buffer: the DATA register address, then
// the data link frame (FCTR, length), then the transport layer packet
// control byte, then a packet fragment. The frame CRC follows the fragment.
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

	// Command and response packets are staged in these buffers so they
	// can span several frames (packet chaining). 700 bytes each fits
	// reading or writing data objects such as the device certificate in
	// a few pieces, at a cost of 1.4kB of RAM per Device.
	txApdu [maxPacketSize]byte
	rxApdu [maxPacketSize]byte

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

	copy(d.txApdu[apduHeaderSize:], applicationID[:])
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

	d.txApdu[apduHeaderSize] = byte(request >> 8)
	d.txApdu[apduHeaderSize+1] = byte(request)
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
	if len(data) > len(d.rxApdu)-apduHeaderSize {
		return 0, errTooLong
	}

	in := d.txApdu[apduHeaderSize:]
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

// SetDataObject writes data into a data object at the given byte offset.
// A single write is limited to 692 bytes; larger objects can be written
// in pieces using offset.
func (d *Device) SetDataObject(oid uint16, offset uint16, data []byte) error {
	if d.frameSize == 0 {
		return errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	if len(data) > len(in)-4 {
		return errTooLong
	}
	in[0] = byte(oid >> 8)
	in[1] = byte(oid)
	in[2] = byte(offset >> 8)
	in[3] = byte(offset)
	copy(in[4:], data)
	_, err := d.command(CMD_SET_DATA_OBJECT, paramWriteData, 4+len(data))
	return err
}

// CalcHash computes the SHA-256 digest of data on the chip. Long messages
// are fed to the chip in parts, so data may be arbitrarily large.
func (d *Device) CalcHash(data []byte) (digest [32]byte, err error) {
	if d.frameSize == 0 {
		return digest, errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	maxChunk := len(in) - 3 // a sequence tag and 16-bit length per part
	for sent := 0; ; {
		chunk, seq := len(data)-sent, uint8(hashStartAndFinal)
		switch {
		case sent == 0 && chunk <= maxChunk:
			// hashStartAndFinal: the whole message in one command
		case sent == 0:
			seq, chunk = hashStart, maxChunk
		case chunk > maxChunk:
			seq, chunk = hashContinue, maxChunk
		default:
			seq = hashFinal
		}
		in[0] = seq
		in[1] = byte(chunk >> 8)
		in[2] = byte(chunk)
		copy(in[3:], data[sent:sent+chunk])
		out, err := d.command(CMD_CALC_HASH, paramSHA256, 3+chunk)
		if err != nil {
			return digest, err
		}
		sent += chunk

		if seq != hashStartAndFinal && seq != hashFinal {
			continue
		}
		// The last part is answered with the digest in a TLV
		if len(out) < 3+len(digest) || out[0] != tagDigestOut ||
			int(out[1])<<8|int(out[2]) != len(digest) {
			return digest, errShortResponse
		}
		copy(digest[:], out[3:])
		return digest, nil
	}
}

// CalcSign signs a digest - typically a SHA-256 hash, e.g. from CalcHash -
// with the ECDSA private key stored in keyOID, whose usage must include
// KeyUsageSign. The signature is written to sig and its length returned.
//
// The chip encodes the signature as the two DER INTEGERs r and s
// concatenated, without the outer SEQUENCE most ECDSA tooling expects, so
// the caller may need to prepend one. A P-256 signature is at most 70
// bytes, a P-384 signature at most 102.
func (d *Device) CalcSign(keyOID uint16, digest, sig []byte) (int, error) {
	if d.frameSize == 0 {
		return 0, errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	if len(digest) > len(in)-8 {
		return 0, errTooLong
	}
	n := putTLV(in, 0, tagDigest, digest)
	n = putTLVWord(in, n, tagSignKey, keyOID)
	out, err := d.command(CMD_CALC_SIGN, paramECDSA, n)
	if err != nil {
		return 0, err
	}
	if len(out) > len(sig) {
		return 0, errShortBuffer
	}
	return copy(sig, out), nil
}

// VerifySign checks an ECDSA signature over a digest against a public key
// supplied by the caller: the DER BIT STRING encoding of an uncompressed
// EC point, as returned by GenKeyPair. The signature must be encoded as
// CalcSign produces it: the two DER INTEGERs r and s without an outer
// SEQUENCE. A signature that does not match yields an error.
func (d *Device) VerifySign(curve Curve, publicKey, digest, sig []byte) error {
	if d.frameSize == 0 {
		return errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	if len(digest)+len(sig)+len(publicKey) > len(in)-13 {
		return errTooLong
	}
	n := putTLV(in, 0, tagDigest, digest)
	n = putTLV(in, n, tagSignature, sig)
	n = putTLVByte(in, n, tagAlgorithm, uint8(curve))
	n = putTLV(in, n, tagPublicKey, publicKey)
	_, err := d.command(CMD_VERIFY_SIGN, paramECDSA, n)
	return err
}

// GenKeyPair generates an ECC key pair on the chip. The private key is
// stored in keyOID (e.g. OID_USER_KEY_1) and never leaves the chip; the
// public key is written to pub and its length returned. The public key is
// encoded as a DER BIT STRING holding the uncompressed EC point: 68 bytes
// (0x03, 0x42, 0x00, 0x04, X, Y) for P-256, 100 bytes for P-384.
func (d *Device) GenKeyPair(keyOID uint16, curve Curve, usage KeyUsage, pub []byte) (int, error) {
	if d.frameSize == 0 {
		return 0, errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	n := putTLVWord(in, 0, tagPrivateKey, keyOID)
	n = putTLVByte(in, n, tagKeyUsage, uint8(usage))
	out, err := d.command(CMD_GEN_KEYPAIR, uint8(curve), n)
	if err != nil {
		return 0, err
	}
	if len(out) < 3 || out[0] != tagPublicKeyOut {
		return 0, errShortResponse
	}
	keyLen := int(out[1])<<8 | int(out[2])
	if 3+keyLen > len(out) {
		return 0, errShortResponse
	}
	if keyLen > len(pub) {
		return 0, errShortBuffer
	}
	return copy(pub, out[3:3+keyLen]), nil
}

// ECDH computes a Diffie-Hellman shared secret from the private key in
// keyOID, whose usage must include KeyUsageKeyAgree, and the peer's public
// key: a DER BIT STRING as returned by GenKeyPair. The secret - the X
// coordinate of the shared point, 32 bytes for P-256 - is written to
// secret and its length returned.
//
// Beware: this driver does not implement the shielded connection, so the
// chip returns the shared secret over the I2C bus unencrypted, readable
// by anyone able to probe the bus.
func (d *Device) ECDH(keyOID uint16, curve Curve, peerPublicKey, secret []byte) (int, error) {
	if d.frameSize == 0 {
		return 0, errNotConfigured
	}
	in := d.txApdu[apduHeaderSize:]
	if len(peerPublicKey) > len(in)-12 {
		return 0, errTooLong
	}
	n := putTLVWord(in, 0, tagPrivateKey, keyOID)
	n = putTLVByte(in, n, tagAlgorithm, uint8(curve))
	n = putTLV(in, n, tagPublicKey, peerPublicKey)
	n = putTLVHeader(in, n, tagExport, 0)
	out, err := d.command(CMD_CALC_SSEC, paramECDH, n)
	if err != nil {
		return 0, err
	}
	if len(out) > len(secret) {
		return 0, errShortBuffer
	}
	return copy(secret, out), nil
}

// putTLVHeader writes a TLV header (tag, 16-bit big-endian length) into
// buf at index i and returns the index just past it
func putTLVHeader(buf []byte, i int, tag uint8, length int) int {
	buf[i] = tag
	buf[i+1] = byte(length >> 8)
	buf[i+2] = byte(length)
	return i + 3
}

// putTLV writes a full TLV into buf at index i and returns the index just
// past it
func putTLV(buf []byte, i int, tag uint8, value []byte) int {
	i = putTLVHeader(buf, i, tag, len(value))
	return i + copy(buf[i:], value)
}

// putTLVWord writes a TLV holding one 16-bit big-endian value, e.g. an OID
func putTLVWord(buf []byte, i int, tag uint8, value uint16) int {
	i = putTLVHeader(buf, i, tag, 2)
	buf[i] = byte(value >> 8)
	buf[i+1] = byte(value)
	return i + 2
}

// putTLVByte writes a TLV holding a single byte, e.g. an algorithm
// identifier
func putTLVByte(buf []byte, i int, tag, value uint8) int {
	i = putTLVHeader(buf, i, tag, 1)
	buf[i] = value
	return i + 1
}

// command sends the APDU whose InData was already placed in
// d.txApdu[apduHeaderSize:] and returns the OutData of the response APDU,
// valid until the next operation
func (d *Device) command(cmd, param uint8, inLen int) ([]byte, error) {
	d.txApdu[0] = cmd
	d.txApdu[1] = param
	d.txApdu[2] = byte(inLen >> 8)
	d.txApdu[3] = byte(inLen)

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

// transceive sends the command packet staged in d.txApdu, fragmented
// across as many data link frames as needed, then receives the response
// packet, reassembling chained frames. It returns the response APDU,
// valid until the next operation.
func (d *Device) transceive(packetLen int) ([]byte, error) {
	if d.frameSize == 0 {
		return nil, errNotConfigured
	}
	// Packet bytes carried per frame, after the transport layer PCTR
	maxData := int(d.frameSize) - dlOverhead - 1

	for sent := 0; ; {
		pctr, chunk := uint8(pctrChainLast), packetLen-sent
		switch {
		case sent == 0 && chunk <= maxData:
			pctr = pctrChainNone
		case sent == 0:
			pctr, chunk = pctrChainFirst, maxData
		case chunk > maxData:
			pctr, chunk = pctrChainIntermediate, maxData
		}
		err := d.sendDataFrame(pctr, d.txApdu[sent:sent+chunk])
		if err != nil {
			return nil, err
		}
		sent += chunk
		if sent == packetLen {
			break
		}
		// Each fragment but the last must be acknowledged with a
		// control frame before the next may be sent
		control, _, err := d.nextFrame()
		if err != nil {
			return nil, err
		}
		if !control {
			return nil, errBadFrame
		}
	}
	return d.receiveResponse(maxData)
}

// sendDataFrame wraps one packet fragment in a data link frame and writes
// it to the DATA register
func (d *Device) sendDataFrame(pctr uint8, fragment []byte) error {
	payloadLen := uint16(len(fragment)) + 1 // transport layer PCTR byte

	d.txSeq = (d.txSeq + 1) & fctrAckNrMask
	d.txBuf[0] = REG_DATA
	d.txBuf[fctrOffset] = d.txSeq<<fctrFrameNrPos | d.rxSeq // seqctr: ACK
	d.txBuf[fctrOffset+1] = byte(payloadLen >> 8)
	d.txBuf[fctrOffset+2] = byte(payloadLen)
	d.txBuf[pctrOffset] = pctr
	copy(d.txBuf[apduOffset:], fragment)
	crc := crc16(d.txBuf[fctrOffset : pctrOffset+payloadLen])
	d.txBuf[pctrOffset+payloadLen] = byte(crc >> 8)
	d.txBuf[pctrOffset+payloadLen+1] = byte(crc)

	return d.tx(d.txBuf[:1+dlOverhead+int(payloadLen)], nil)
}

// receiveResponse collects the response packet: the chip acknowledges our
// last frame either with a control frame or piggybacked on its response,
// which arrives in one frame or as a chain of fragments. Chained
// fragments are reassembled into d.rxApdu; an unchained response is
// returned straight out of the frame buffer.
func (d *Device) receiveResponse(maxData int) ([]byte, error) {
	total, chained := 0, false
	for controlFrames := 0; ; {
		control, n, err := d.nextFrame()
		if err != nil {
			return nil, err
		}
		if control {
			if controlFrames++; controlFrames > 4 {
				return nil, errTimeout
			}
			continue
		}
		controlFrames = 0

		fragment := d.rxBuf[4 : n-2]
		pctr := d.rxBuf[3] & pctrChainMask
		switch {
		case pctr == pctrChainNone && !chained:
			return fragment, nil
		case (pctr == pctrChainFirst && !chained) ||
			(pctr == pctrChainIntermediate && chained):
			// Every fragment before the last fills its frame
			if len(fragment) != maxData {
				return nil, errBrokenChain
			}
			if total+len(fragment) > len(d.rxApdu) {
				return nil, errTooLong
			}
			total += copy(d.rxApdu[total:], fragment)
			chained = true
		case pctr == pctrChainLast && chained:
			if total+len(fragment) > len(d.rxApdu) {
				return nil, errTooLong
			}
			total += copy(d.rxApdu[total:], fragment)
			return d.rxApdu[:total], nil
		default:
			return nil, errBrokenChain
		}
	}
}

// nextFrame reads one frame and validates its acknowledgement fields,
// reporting whether it was a control frame. Data frames are sequence
// checked and acknowledged before returning.
func (d *Device) nextFrame() (control bool, n int, err error) {
	n, err = d.readFrame()
	if err != nil {
		return false, 0, err
	}
	fctr := d.rxBuf[0]
	if fctr&fctrAckNrMask != d.txSeq {
		return false, 0, errUnexpectedAck
	}
	seqctr := (fctr & fctrSeqctrMask) >> fctrSeqctrPos
	if seqctr == seqctrNack {
		return false, 0, errNack
	}
	if seqctr != seqctrAck {
		return false, 0, errBadFrame
	}
	if fctr&fctrControlFrame != 0 {
		return true, n, nil
	}
	if (fctr&fctrFrameNrMask)>>fctrFrameNrPos != (d.rxSeq+1)&fctrAckNrMask {
		return false, 0, errBadFrame
	}
	d.rxSeq = (d.rxSeq + 1) & fctrAckNrMask
	return false, n, d.sendAck()
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
