package trustm

import (
	"bytes"
	"errors"
	"testing"
)

// TestCRC16 pins the frame check sequence to known vectors. The Infineon
// algorithm is CRC-16/KERMIT, whose published check value for "123456789"
// is 0x2189. The second vector is the OpenApplication example frame from
// the Infineon I2C protocol specification, whose CRC is 0x041A.
func TestCRC16(t *testing.T) {
	if got := crc16([]byte("123456789")); got != 0x2189 {
		t.Errorf("crc16(check vector) = %#04X, want 0x2189", got)
	}

	frame := []byte{
		0x03, 0x00, 0x15, 0x00, 0x70, 0x00, 0x00, 0x10,
		0xD2, 0x76, 0x00, 0x00, 0x04, 0x47, 0x65, 0x6E,
		0x41, 0x75, 0x74, 0x68, 0x41, 0x70, 0x70, 0x6C,
	}
	if got := crc16(frame); got != 0x041A {
		t.Errorf("crc16(OpenApplication frame) = %#04X, want 0x041A", got)
	}
}

// TestGetRandomFrame checks the exact bytes written to the DATA register
// for a GetRandom command on a fresh link: frame control 0x03 (frame 0,
// acknowledging 3), payload length 7, packet control 0, APDU, CRC
func TestGetRandomFrame(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	d.frameSize = 0x110
	if err := d.GetRandom(make([]byte, 16)); err != nil {
		t.Fatal(err)
	}

	want := []byte{
		0x80, // DATA register
		0x03, 0x00, 0x07,
		0x00,                               // PCTR: no chaining
		0x8C, 0x00, 0x00, 0x02, 0x00, 0x10, // GetRandom TRNG, 16 bytes
		0x79, 0x08, // CRC
	}
	if len(chip.frameWrites) == 0 || !bytes.Equal(chip.frameWrites[0], want) {
		t.Errorf("frame written = %X, want %X", chip.frameWrites[0], want)
	}
}

func TestConfigureAndCommands(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)

	if !d.Connected() {
		t.Error("expected device to be connected")
	}
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	if !chip.opened {
		t.Error("Configure did not open the application")
	}

	rnd := make([]byte, 16)
	if err := d.GetRandom(rnd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rnd, chip.random[:16]) {
		t.Errorf("GetRandom() = %X, want %X", rnd, chip.random[:16])
	}

	uid, err := d.UID()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uid[:], chip.uid[:]) {
		t.Errorf("UID() = %X, want %X", uid, chip.uid)
	}
}

func TestGetRandomLengthLimits(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}

	// Below the chip's 8-byte minimum, the driver requests more and
	// truncates
	rnd := make([]byte, 3)
	if err := d.GetRandom(rnd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rnd, chip.random[:3]) {
		t.Errorf("GetRandom() = %X, want %X", rnd, chip.random[:3])
	}

	if err := d.GetRandom(make([]byte, 257)); err == nil {
		t.Error("expected an error for a 257 byte request")
	}
}

func TestCommandsBeforeConfigure(t *testing.T) {
	d := New(newFakeChip())
	if err := d.GetRandom(make([]byte, 16)); err == nil {
		t.Error("expected an error before Configure")
	}
}

// TestChainedResponse reads a data object bigger than one frame: the fake
// chip chains the response across three frames, released one by one as
// the driver acknowledges them, and the driver reassembles the packet
func TestChainedResponse(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}
	acks := chip.hostAcks

	var buf [600]byte
	n, err := d.GetDataObject(OID_DEVICE_CERTIFICATE, 0, buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if n != len(chip.cert) || !bytes.Equal(buf[:n], chip.cert[:]) {
		t.Errorf("GetDataObject() read %d bytes, want the %d byte object",
			n, len(chip.cert))
	}
	// The 604 byte response travels as chained frames of 266+266+72
	// bytes, each acknowledged individually
	if got := chip.hostAcks - acks; got != 3 {
		t.Errorf("host acknowledged %d frames, want 3", got)
	}
}

// TestChainedCommand writes a data object bigger than one frame: the
// driver must fragment the command packet into a chain of frames, which
// the fake chip validates and reassembles
func TestChainedCommand(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 600)
	for i := range data {
		data[i] = byte(i)
	}
	if err := d.SetDataObject(0xF1D0, 7, data); err != nil {
		t.Fatal(err)
	}
	if chip.setOID != 0xF1D0 || chip.setOffset != 7 || !bytes.Equal(chip.setData, data) {
		t.Errorf("chip received OID %#04X, offset %d, %d bytes; want 0xF1D0, 7, the original data",
			chip.setOID, chip.setOffset, len(chip.setData))
	}

	packet := chip.packets[len(chip.packets)-1]
	header := []byte{
		0x82, 0x00, 0x02, 0x5C, // SetDataObject, write data, InLen 604
		0xF1, 0xD0, 0x00, 0x07, // OID, offset
	}
	if !bytes.Equal(packet[:len(header)], header) {
		t.Errorf("SetDataObject APDU header = %X, want %X", packet[:len(header)], header)
	}
}

// TestCryptoCommandAPDUs pins the exact command APDUs of the
// cryptographic commands to the TLV layouts given in the Solution
// Reference Manual, and checks their responses are unpacked correctly
func TestCryptoCommandAPDUs(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}

	digest, err := d.CalcHash([]byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if digest != chip.digest {
		t.Errorf("CalcHash() = %X, want %X", digest, chip.digest)
	}
	want := []byte{
		0xB0, 0xE2, 0x00, 0x06, // CalcHash, SHA-256, InLen 6
		0x01, 0x00, 0x03, 'a', 'b', 'c', // start&final, 3 message bytes
	}
	assertPacket(t, "CalcHash", chip, want)

	var sig [80]byte
	sigLen, err := d.CalcSign(OID_USER_KEY_1, digest[:], sig[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sig[:sigLen], chip.sig[:]) {
		t.Errorf("CalcSign() = %X, want %X", sig[:sigLen], chip.sig)
	}
	want = []byte{
		0xB1, 0x11, 0x00, 0x28, // CalcSign, ECDSA, InLen 40
		0x01, 0x00, 0x20, // 32 byte digest
	}
	want = append(want, digest[:]...)
	want = append(want, 0x03, 0x00, 0x02, 0xE0, 0xF1) // signature key OID
	assertPacket(t, "CalcSign", chip, want)

	if err := d.VerifySign(P256, chip.pubKey[:], digest[:], sig[:sigLen]); err != nil {
		t.Fatal(err)
	}
	want = []byte{
		0xB2, 0x11, 0x00, 0xB7, // VerifySign, ECDSA, InLen 183
		0x01, 0x00, 0x20, // 32 byte digest
	}
	want = append(want, digest[:]...)
	want = append(want, 0x02, 0x00, byte(sigLen)) // the signature
	want = append(want, sig[:sigLen]...)
	want = append(want, 0x05, 0x00, 0x01, 0x03)             // algorithm: P-256
	want = append(want, 0x06, 0x00, byte(len(chip.pubKey))) // public key
	want = append(want, chip.pubKey[:]...)
	assertPacket(t, "VerifySign", chip, want)

	var pub [80]byte
	pubLen, err := d.GenKeyPair(OID_USER_KEY_2, P256, KeyUsageSign|KeyUsageAuth, pub[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub[:pubLen], chip.pubKey[:]) {
		t.Errorf("GenKeyPair() = %X, want %X", pub[:pubLen], chip.pubKey)
	}
	want = []byte{
		0xB8, 0x03, 0x00, 0x09, // GenKeyPair, P-256, InLen 9
		0x01, 0x00, 0x02, 0xE0, 0xF2, // private key OID
		0x02, 0x00, 0x01, 0x11, // key usage: sign | auth
	}
	assertPacket(t, "GenKeyPair", chip, want)

	var secret [32]byte
	secretLen, err := d.ECDH(OID_USER_KEY_2, P256, pub[:pubLen], secret[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret[:secretLen], chip.secret[:]) {
		t.Errorf("ECDH() = %X, want %X", secret[:secretLen], chip.secret)
	}
	want = []byte{
		0xB3, 0x01, 0x00, 0x53, // CalcSSec, ECDH, InLen 83
		0x01, 0x00, 0x02, 0xE0, 0xF2, // private key OID
		0x05, 0x00, 0x01, 0x03, // algorithm: P-256
		0x06, 0x00, 0x44, // 68 byte peer public key
	}
	want = append(want, pub[:pubLen]...)
	want = append(want, 0x07, 0x00, 0x00) // export the secret
	assertPacket(t, "ECDH", chip, want)
}

func assertPacket(t *testing.T, name string, chip *fakeChip, want []byte) {
	t.Helper()
	if len(chip.packets) == 0 {
		t.Fatalf("%s: no command packet received", name)
	}
	if got := chip.packets[len(chip.packets)-1]; !bytes.Equal(got, want) {
		t.Errorf("%s APDU = %X, want %X", name, got, want)
	}
}

// TestCalcHashChunking hashes a message too big for one command packet:
// the driver must feed it to the chip as a start-continue-final sequence
func TestCalcHashChunking(t *testing.T) {
	chip := newFakeChip()
	d := New(chip)
	if err := d.Configure(); err != nil {
		t.Fatal(err)
	}

	// Each CalcHash command carries at most 693 message bytes
	data := make([]byte, 1500)
	for i := range data {
		data[i] = byte(i >> 3)
	}
	digest, err := d.CalcHash(data)
	if err != nil {
		t.Fatal(err)
	}
	if digest != chip.digest {
		t.Errorf("CalcHash() = %X, want %X", digest, chip.digest)
	}

	parts := chip.packets[len(chip.packets)-3:]
	sequence := []byte{hashStart, hashContinue, hashFinal}
	lengths := []int{693, 693, 114}
	rebuilt := []byte{}
	for i, part := range parts {
		if part[0] != CMD_CALC_HASH || part[4] != sequence[i] ||
			int(part[5])<<8|int(part[6]) != lengths[i] {
			t.Errorf("part %d starts %X, want command B0 sequence %02X length %d",
				i, part[:7], sequence[i], lengths[i])
		}
		rebuilt = append(rebuilt, part[7:]...)
	}
	if !bytes.Equal(rebuilt, data) {
		t.Error("hash parts do not add up to the original message")
	}
}

// The fake chip reports a frame size of 0x110, so it carries this many
// packet bytes per frame after the PCTR byte
const chipMaxData = 0x110 - dlOverhead - 1

// fakeChip emulates the OPTIGA Trust M I2C register set and protocol: it
// ACKs valid data frames, reassembles chained command packets and chains
// long responses, answers the implemented commands, and exposes what was
// written for inspection
type fakeChip struct {
	lastReg     byte     // register selected by the last 1-byte write
	pending     [][]byte // frames waiting to be read from DATA
	seq         uint8    // the chip's own frame sequence counter
	opened      bool
	frameWrites [][]byte // every write to the DATA register, with register byte
	packets     [][]byte // every reassembled command packet (APDU)
	hostAcks    int      // control frames received from the host

	rxPacket    []byte // partial command packet while the host chains
	resp        []byte // response bytes not yet framed
	respStarted bool   // a chained response is under way
	lastHostSeq uint8  // sequence number the response frames acknowledge

	random [maxRandomLength]byte
	uid    [UIDLength]byte
	cert   [600]byte // outsized data object, read back in chained frames
	digest [32]byte
	pubKey [68]byte
	sig    [70]byte
	secret [32]byte

	setOID    uint16 // what the last SetDataObject wrote
	setOffset uint16
	setData   []byte
}

func newFakeChip() *fakeChip {
	f := &fakeChip{seq: seqInit}
	for i := range f.random {
		f.random[i] = byte(0xA5 ^ i)
	}
	for i := range f.uid {
		f.uid[i] = byte(0xC0 + i)
	}
	for i := range f.cert {
		f.cert[i] = byte(0x30 + i)
	}
	for i := range f.digest {
		f.digest[i] = byte(0xD0 ^ i)
	}
	for i := range f.pubKey {
		f.pubKey[i] = byte(0x50 + i)
	}
	for i := range f.sig {
		f.sig[i] = byte(0x90 - i)
	}
	for i := range f.secret {
		f.secret[i] = byte(0x11 * i)
	}
	return f
}

func (f *fakeChip) Tx(addr uint16, w, r []byte) error {
	if addr != Address {
		return errors.New("NACK: unknown I2C address")
	}
	if len(w) > 0 {
		f.lastReg = w[0]
		if f.lastReg == REG_DATA && len(w) > 1 {
			f.frameWrites = append(f.frameWrites, append([]byte(nil), w...))
			return f.receiveFrame(w[1:])
		}
		return nil
	}

	switch f.lastReg {
	case REG_I2C_STATE:
		r[0] = stateSoftReset
		if len(f.pending) > 0 {
			r[0] |= stateResponseReady
			r[2] = byte(len(f.pending[0]) >> 8)
			r[3] = byte(len(f.pending[0]))
		}
	case REG_DATA_REG_LEN:
		r[0], r[1] = 0x01, 0x10
	case REG_DATA:
		if len(f.pending) == 0 {
			return errors.New("NACK: no frame pending")
		}
		copy(r, f.pending[0])
		f.pending = f.pending[1:]
	}
	return nil
}

func (f *fakeChip) receiveFrame(frame []byte) error {
	if len(frame) < dlOverhead {
		return errors.New("frame too short")
	}
	crc := uint16(frame[len(frame)-2])<<8 | uint16(frame[len(frame)-1])
	if crc16(frame[:len(frame)-2]) != crc {
		return errors.New("bad frame CRC")
	}
	fctr := frame[0]
	if fctr&fctrControlFrame != 0 {
		// An ACK for our last data frame; it releases the next
		// fragment of a chained response
		f.hostAcks++
		f.nextResponseFragment()
		return nil
	}

	hostSeq := fctr >> fctrFrameNrPos & fctrAckNrMask
	payload := frame[4 : len(frame)-2]

	// Reassemble chained command packets, validating the chain sequence
	// like ifx_i2c_transport_layer.c does
	switch frame[3] & pctrChainMask {
	case pctrChainNone:
		if f.rxPacket != nil {
			return errors.New("chain interrupted by unchained frame")
		}
	case pctrChainFirst:
		if f.rxPacket != nil || len(payload) != chipMaxData {
			return errors.New("bad first chain fragment")
		}
	case pctrChainIntermediate:
		if f.rxPacket == nil || len(payload) != chipMaxData {
			return errors.New("bad intermediate chain fragment")
		}
	case pctrChainLast:
		if f.rxPacket == nil {
			return errors.New("last chain fragment without first")
		}
	default:
		return errors.New("unexpected packet control byte")
	}
	f.rxPacket = append(f.rxPacket, payload...)

	// Acknowledge the data frame with a control frame
	f.lastHostSeq = hostSeq
	f.queueFrame(fctrControlFrame|hostSeq, 0, nil)

	switch frame[3] & pctrChainMask {
	case pctrChainFirst, pctrChainIntermediate:
		return nil // more fragments to come
	}

	apdu := f.rxPacket
	f.rxPacket = nil
	f.packets = append(f.packets, apdu)
	out, err := f.handle(apdu)
	if err != nil {
		return err
	}
	f.resp = append([]byte{0x00, 0x00, byte(len(out) >> 8), byte(len(out))}, out...)
	f.nextResponseFragment()
	return nil
}

// handle answers one command APDU with its OutData
func (f *fakeChip) handle(apdu []byte) ([]byte, error) {
	switch apdu[0] {
	case CMD_OPEN_APPLICATION:
		f.opened = true
		return nil, nil
	case CMD_GET_RANDOM:
		return f.random[:int(apdu[4])<<8|int(apdu[5])], nil
	case CMD_GET_DATA_OBJECT:
		var obj []byte
		switch uint16(apdu[4])<<8 | uint16(apdu[5]) {
		case OID_COPROCESSOR_UID:
			obj = f.uid[:]
		case OID_DEVICE_CERTIFICATE:
			obj = f.cert[:]
		default:
			return nil, errors.New("unexpected OID")
		}
		offset := int(apdu[6])<<8 | int(apdu[7])
		n := int(apdu[8])<<8 | int(apdu[9])
		if offset+n > len(obj) {
			n = len(obj) - offset
		}
		return obj[offset : offset+n], nil
	case CMD_SET_DATA_OBJECT:
		f.setOID = uint16(apdu[4])<<8 | uint16(apdu[5])
		f.setOffset = uint16(apdu[6])<<8 | uint16(apdu[7])
		f.setData = append([]byte(nil), apdu[8:]...)
		return nil, nil
	case CMD_CALC_HASH:
		if seq := apdu[4]; seq == hashStartAndFinal || seq == hashFinal {
			out := []byte{tagDigestOut, 0x00, byte(len(f.digest))}
			return append(out, f.digest[:]...), nil
		}
		return nil, nil
	case CMD_CALC_SIGN:
		return f.sig[:], nil
	case CMD_VERIFY_SIGN:
		return nil, nil
	case CMD_GEN_KEYPAIR:
		out := []byte{tagPublicKeyOut, byte(len(f.pubKey) >> 8), byte(len(f.pubKey))}
		return append(out, f.pubKey[:]...), nil
	case CMD_CALC_SSEC:
		return f.secret[:], nil
	default:
		return nil, errors.New("unexpected command")
	}
}

// nextResponseFragment queues the next frame of the pending response: the
// whole response if it fits, otherwise the next full-sized fragment of
// the chain, held back until the host acknowledged the previous one
func (f *fakeChip) nextResponseFragment() {
	if f.resp == nil {
		return
	}
	pctr, chunk := byte(pctrChainNone), len(f.resp)
	switch {
	case !f.respStarted && chunk <= chipMaxData:
		// a single unchained frame
	case !f.respStarted:
		pctr, chunk = pctrChainFirst, chipMaxData
	case chunk > chipMaxData:
		pctr, chunk = pctrChainIntermediate, chipMaxData
	default:
		pctr = pctrChainLast
	}
	f.seq = (f.seq + 1) & fctrAckNrMask
	f.queueFrame(f.seq<<fctrFrameNrPos|f.lastHostSeq, pctr, f.resp[:chunk])
	f.respStarted = true
	if f.resp = f.resp[chunk:]; len(f.resp) == 0 {
		f.resp, f.respStarted = nil, false
	}
}

func (f *fakeChip) queueFrame(fctr, pctr byte, payload []byte) {
	var frame []byte
	if payload == nil {
		frame = []byte{fctr, 0x00, 0x00}
	} else {
		payloadLen := 1 + len(payload) // PCTR byte
		frame = append([]byte{fctr, byte(payloadLen >> 8), byte(payloadLen), pctr}, payload...)
	}
	crc := crc16(frame)
	frame = append(frame, byte(crc>>8), byte(crc))
	f.pending = append(f.pending, frame)
}
