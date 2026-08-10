package st25dv

import (
	"errors"
	"strings"
)

// NFC Forum Type 5 Tag layout, so the tag content can be read by NFC
// devices such as smartphones. The user memory starts with a capability
// container (CC), followed by the NDEF message wrapped in a TLV block.
//
// References: NFC Forum "Type 5 Tag" and "URI Record Type Definition"
// specifications, ST application note AN4911 and the ST NDEF library:
// https://github.com/stm32duino/ST25DV/tree/main/src/libNDEF

var (
	errMessageTooLong = errors.New("st25dv: NDEF message does not fit in memory")
	errURITooLong     = errors.New("st25dv: URI too long for a short NDEF record")
)

const (
	// CC byte 0: magic number. E1 when the whole data area is reachable
	// with 1-byte block address RF commands, E2 when the tag has more than
	// 256 blocks and needs the extended (2-byte block address) commands.
	ccMagic1ByteAddress uint8 = 0xE1
	ccMagic2ByteAddress uint8 = 0xE2

	// CC byte 1: NDEF mapping version 1.0, read and write access granted
	ccVersion1dot0FreeAccess uint8 = 0x40

	// CC byte 3: additional features, the value ST's own drivers advertise
	// for the ST25DV family (Read Multiple Block and Lock Block supported)
	ccFeatures uint8 = 0x05

	// TLV block types
	tlvNDEFMessage uint8 = 0x03
	tlvTerminator  uint8 = 0xFE

	// Marker for the 3-byte TLV length format
	tlvLength3Bytes uint8 = 0xFF
)

// ccLength returns the capability container size that fits this tag's
// memory: the compact 4-byte CC can only describe up to 255 8-byte blocks
func (d *Device) ccLength() uint8 {
	if d.memSize/8 > 0xFF {
		return 8
	}
	return 4
}

// FormatNDEF writes the NFC Forum Type 5 Tag capability container and an
// empty NDEF message, erasing whatever message was on the tag
func (d *Device) FormatNDEF() error {
	if d.memSize == 0 {
		return errNotConfigured
	}

	blocks := d.memSize / 8
	var buf [10]byte
	cc := d.ccLength()
	buf[0] = ccMagic1ByteAddress
	buf[1] = ccVersion1dot0FreeAccess
	buf[2] = uint8(blocks)
	buf[3] = ccFeatures
	if cc == 8 {
		buf[0] = ccMagic2ByteAddress
		buf[2] = 0x00 // the 8-byte CC keeps the size in bytes 6-7 instead
		buf[6] = uint8(blocks >> 8)
		buf[7] = uint8(blocks)
	}

	// Empty NDEF message TLV
	buf[cc] = tlvNDEFMessage
	buf[cc+1] = 0x00

	_, err := d.WriteAt(buf[:cc+2], 0)
	return err
}

// WriteNDEFMessage writes a raw NDEF message to the tag, wrapped in the
// Type 5 Tag TLV framing. The capability container is written first in case
// the tag does not have a valid one yet.
//
// The TLV length is written last, after the message body and terminator, so
// an NFC reader polling the tag never sees a partially written message.
func (d *Device) WriteNDEFMessage(msg []byte) error {
	if d.memSize == 0 {
		return errNotConfigured
	}

	var magic [1]byte
	_, err := d.ReadAt(magic[:], 0)
	if err != nil {
		return err
	}

	// The magic byte on the tag, not this driver's own size-based guess,
	// determines where the existing CC ends: a tag formatted elsewhere
	// may legitimately use the other CC width for its capacity, and
	// trusting our own guess would misplace the TLV against the real CC.
	var tlvStart int64
	switch magic[0] {
	case ccMagic1ByteAddress:
		tlvStart = 4
	case ccMagic2ByteAddress:
		tlvStart = 8
	default:
		if err := d.FormatNDEF(); err != nil {
			return err
		}
		tlvStart = int64(d.ccLength())
	}

	lengthSize := int64(1)
	if len(msg) >= int(tlvLength3Bytes) {
		lengthSize = 3
	}
	bodyStart := tlvStart + 1 + lengthSize
	if bodyStart+int64(len(msg))+1 > int64(d.memSize) {
		return errMessageTooLong
	}

	// TLV type with a zeroed length, then the message and terminator
	var header [4]byte
	header[0] = tlvNDEFMessage
	_, err = d.WriteAt(header[:1+lengthSize], tlvStart)
	if err != nil {
		return err
	}
	_, err = d.WriteAt(msg, bodyStart)
	if err != nil {
		return err
	}
	_, err = d.WriteAt([]byte{tlvTerminator}, bodyStart+int64(len(msg)))
	if err != nil {
		return err
	}

	// Reveal the message by writing its real length
	length := header[:1]
	if lengthSize == 3 {
		length = header[:3]
		length[0] = tlvLength3Bytes
		length[1] = uint8(len(msg) >> 8)
		length[2] = uint8(len(msg))
	} else {
		length[0] = uint8(len(msg))
	}
	_, err = d.WriteAt(length, tlvStart+1)
	return err
}

// WriteNDEFURI writes an NDEF message with a single URI record to the tag,
// e.g. a URL a smartphone opens when tapped. Pass the full URI including its
// scheme; well-known prefixes are abbreviated automatically as defined by
// the NFC Forum URI Record Type Definition.
func (d *Device) WriteNDEFURI(uri string) error {
	code, rest := abbreviateURI(uri)
	if len(rest) > 254 {
		return errURITooLong
	}

	msg := make([]byte, 5+len(rest))
	msg[0] = 0xD1 // single record: message begin + end, short record, well-known type
	msg[1] = 0x01 // type length
	msg[2] = uint8(1 + len(rest))
	msg[3] = 'U'
	msg[4] = code
	copy(msg[5:], rest)

	return d.WriteNDEFMessage(msg)
}

// URI abbreviation codes from the NFC Forum URI Record Type Definition
var uriPrefixes = [...]string{
	0x01: "http://www.",
	0x02: "https://www.",
	0x03: "http://",
	0x04: "https://",
	0x05: "tel:",
	0x06: "mailto:",
	0x07: "ftp://anonymous:anonymous@",
	0x08: "ftp://ftp.",
	0x09: "ftps://",
	0x0A: "sftp://",
	0x0B: "smb://",
	0x0C: "nfs://",
	0x0D: "ftp://",
	0x0E: "dav://",
	0x0F: "news:",
	0x10: "telnet://",
	0x11: "imap:",
	0x12: "rtsp://",
	0x13: "urn:",
	0x14: "pop:",
	0x15: "sip:",
	0x16: "sips:",
	0x17: "tftp:",
	0x18: "btspp://",
	0x19: "btl2cap://",
	0x1A: "btgoep://",
	0x1B: "tcpobex://",
	0x1C: "irdaobex://",
	0x1D: "file://",
	0x1E: "urn:epc:id:",
	0x1F: "urn:epc:tag:",
	0x20: "urn:epc:pat:",
	0x21: "urn:epc:raw:",
	0x22: "urn:epc:",
	0x23: "urn:nfc:",
}

// abbreviateURI finds the longest matching well-known prefix and returns its
// code and the remainder of the URI
func abbreviateURI(uri string) (code uint8, rest string) {
	longest := 0
	for c, prefix := range uriPrefixes {
		if len(prefix) > longest && strings.HasPrefix(uri, prefix) {
			longest = len(prefix)
			code = uint8(c)
		}
	}
	return code, uri[longest:]
}
