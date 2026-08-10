// Package signer bridges keys held in an Infineon OPTIGA Trust M into Go's
// standard crypto interfaces: it exposes a chip key slot as a crypto.Signer
// and the factory-provisioned device certificate as an x509.Certificate, so
// the chip can back TLS client certificates and other PKI uses while the
// private key never leaves the chip.
//
// This package pulls in crypto/ecdsa, crypto/x509 and math/big, which cost
// several hundred kilobytes of flash under TinyGo; it is kept separate from
// the trustm driver so that plain driver users do not pay for it.
package signer

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"errors"
	"io"
	"math/big"

	"tinygo.org/x/drivers/trustm"
)

var (
	errUnsupportedCurve = errors.New("trustm/signer: unsupported curve")
	errBadPublicKey     = errors.New("trustm/signer: malformed public key")
	errBadCertificate   = errors.New("trustm/signer: unrecognized certificate object framing")
	errShortObject      = errors.New("trustm/signer: data object ended early")
	errNoPublicKey      = errors.New("trustm/signer: signer has no public key")
	errBadSignature     = errors.New("trustm/signer: malformed signature from the chip")
)

// Chip is the part of the trustm.Device API this package uses; satisfied by
// *trustm.Device.
type Chip interface {
	CalcSign(keyOID uint16, digest, sig []byte) (int, error)
	GenKeyPair(keyOID uint16, curve trustm.Curve, usage trustm.KeyUsage, pub []byte) (int, error)
	GetDataObject(oid uint16, offset uint16, data []byte) (int, error)
}

// Signer signs digests with a private key stored in an OPTIGA Trust M key
// slot. It implements crypto.Signer.
type Signer struct {
	chip   Chip
	keyOID uint16
	pub    *ecdsa.PublicKey
}

// New wraps an existing chip key as a crypto.Signer. The caller must
// provide the matching public key, e.g. parsed from the key's certificate
// or remembered from GenKeyPair; for a freshly generated key use Generate
// instead. Sign needs it to size the digest, so it may not be nil.
func New(chip Chip, keyOID uint16, pub *ecdsa.PublicKey) *Signer {
	return &Signer{chip: chip, keyOID: keyOID, pub: pub}
}

// Generate creates a new ECDSA key pair in the given chip key slot (one of
// trustm.OID_USER_KEY_1 to 3) and returns it as a crypto.Signer. The
// private key is generated on, and never leaves, the chip.
func Generate(chip Chip, keyOID uint16, curve trustm.Curve) (*Signer, error) {
	// A P-384 public key needs 100 bytes in the chip's encoding
	var buf [112]byte
	n, err := chip.GenKeyPair(keyOID, curve, trustm.KeyUsageSign|trustm.KeyUsageAuth, buf[:])
	if err != nil {
		return nil, err
	}
	pub, err := ParsePublicKey(curve, buf[:n])
	if err != nil {
		return nil, err
	}
	return New(chip, keyOID, pub), nil
}

// Public returns the public key matching the chip's private key.
func (s *Signer) Public() crypto.PublicKey {
	return s.pub
}

// Sign signs digest with the chip's private key and returns an ECDSA
// signature in the ASN.1 SEQUENCE form Go's crypto packages expect
// (crypto/ecdsa VerifyASN1, crypto/tls, crypto/x509).
//
// A digest longer than the key's curve is truncated to its leftmost bytes,
// as ECDSA prescribes; the chip refuses to sign an oversized one. The rand
// and opts parameters are ignored: the chip uses its internal true random
// number generator, and signs the digest exactly as given.
func (s *Signer) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	if s.pub == nil {
		return nil, errNoPublicKey
	}
	if size := (s.pub.Curve.Params().BitSize + 7) / 8; len(digest) > size {
		digest = digest[:size]
	}

	// The chip returns the two DER INTEGERs r and s without the outer
	// SEQUENCE: at most 2+49 bytes each for P-384
	var raw [112]byte
	n, err := s.chip.CalcSign(s.keyOID, digest, raw[:])
	if err != nil {
		return nil, err
	}
	return asn1Signature(raw[:n])
}

// asn1Signature adds the outer SEQUENCE the chip leaves off its two DER
// INTEGERs r and s.
//
// The integers are re-encoded rather than copied because the chip sometimes
// emits a leading zero byte that the value does not need. That is not valid
// DER and Go's ASN.1 readers reject it outright, so Infineon's host library
// strips it too (optiga_cmd_ecc_r_s_padding_check in optiga_cmd.c).
func asn1Signature(chipSig []byte) ([]byte, error) {
	r, rest, err := splitInteger(chipSig)
	if err != nil {
		return nil, err
	}
	s, rest, err := splitInteger(rest)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errBadSignature
	}

	// r and s hold at most 48 bytes each on the curves the chip supports, so
	// the short length form always suffices
	body := append(derInteger(r), derInteger(s)...)
	if len(body) > 0x7F {
		return nil, errBadSignature
	}
	return append([]byte{0x30, byte(len(body))}, body...), nil
}

// splitInteger takes the leading DER INTEGER off buf, returning its value
// and whatever follows it
func splitInteger(buf []byte) (value, rest []byte, err error) {
	if len(buf) < 3 || buf[0] != 0x02 || buf[1] == 0 || 2+int(buf[1]) > len(buf) {
		return nil, nil, errBadSignature
	}
	return buf[2 : 2+buf[1]], buf[2+buf[1]:], nil
}

// derInteger encodes a non-negative big-endian integer as a minimal DER
// INTEGER: no redundant leading zeroes, one zero byte added when the leading
// bit would otherwise mark the value as negative
func derInteger(value []byte) []byte {
	for len(value) > 1 && value[0] == 0x00 {
		value = value[1:]
	}
	if value[0]&0x80 != 0 {
		return append([]byte{0x02, byte(len(value) + 1), 0x00}, value...)
	}
	return append([]byte{0x02, byte(len(value))}, value...)
}

// ellipticCurve maps the chip's curve identifiers onto Go's curves
func ellipticCurve(curve trustm.Curve) (elliptic.Curve, error) {
	switch curve {
	case trustm.P256:
		return elliptic.P256(), nil
	case trustm.P384:
		return elliptic.P384(), nil
	}
	return nil, errUnsupportedCurve
}

// ParsePublicKey converts an ECC public key in the chip's encoding - the
// DER BIT STRING holding an uncompressed EC point, as produced by
// trustm.Device.GenKeyPair - into an ecdsa.PublicKey.
func ParsePublicKey(curve trustm.Curve, chipKey []byte) (*ecdsa.PublicKey, error) {
	c, err := ellipticCurve(curve)
	if err != nil {
		return nil, err
	}
	size := (c.Params().BitSize + 7) / 8

	// 03 <len> 00 04 X Y: BIT STRING tag, length, no unused bits,
	// uncompressed point
	if len(chipKey) != 4+2*size || chipKey[0] != 0x03 ||
		int(chipKey[1]) != 2+2*size || chipKey[2] != 0x00 || chipKey[3] != 0x04 {
		return nil, errBadPublicKey
	}

	x := new(big.Int).SetBytes(chipKey[4 : 4+size])
	y := new(big.Int).SetBytes(chipKey[4+size:])
	if !c.IsOnCurve(x, y) {
		return nil, errBadPublicKey
	}
	return &ecdsa.PublicKey{Curve: c, X: x, Y: y}, nil
}

// MarshalPublicKey is the inverse of ParsePublicKey: it converts an
// ecdsa.PublicKey into the chip's encoding, as accepted by
// trustm.Device.VerifySign and ECDH for external public keys.
func MarshalPublicKey(pub *ecdsa.PublicKey) ([]byte, error) {
	var size int
	switch pub.Curve {
	case elliptic.P256():
		size = 32
	case elliptic.P384():
		size = 48
	default:
		return nil, errUnsupportedCurve
	}

	key := make([]byte, 4+2*size)
	key[0] = 0x03
	key[1] = byte(2 + 2*size)
	key[2] = 0x00
	key[3] = 0x04
	pub.X.FillBytes(key[4 : 4+size])
	pub.Y.FillBytes(key[4+size:])
	return key, nil
}

// Data objects are read in pieces this big, comfortably below the trustm
// driver's per-read limit
const readChunk = 512

// DeviceCertificate reads and parses the X.509 certificate that Infineon
// provisions in trustm.OID_DEVICE_CERTIFICATE, the counterpart to the
// private key in trustm.OID_DEVICE_KEY. Both storage framings from the
// Solution Reference Manual are handled: a plain DER certificate, and the
// "TLS identity" chain wrapper (tag 0xC0), of which the first certificate
// is returned.
func DeviceCertificate(chip Chip) (*x509.Certificate, error) {
	return Certificate(chip, trustm.OID_DEVICE_CERTIFICATE)
}

// Certificate reads and parses an X.509 certificate from any data object;
// see DeviceCertificate.
func Certificate(chip Chip, oid uint16) (*x509.Certificate, error) {
	var head [9]byte
	n, err := chip.GetDataObject(oid, 0, head[:])
	if err != nil {
		return nil, err
	}
	// A shorter answer means the object holds less than a framing header, so
	// it is empty rather than holding a certificate
	if n < len(head) {
		return nil, errShortObject
	}

	var start, length int
	switch head[0] {
	case 0x30:
		// A plain DER certificate; its total size comes from the outer
		// SEQUENCE header
		switch {
		case head[1] < 0x80:
			start, length = 0, 2+int(head[1])
		case head[1] == 0x81:
			start, length = 0, 3+int(head[2])
		case head[1] == 0x82:
			start, length = 0, 4+int(head[2])<<8+int(head[3])
		default:
			return nil, errBadCertificate
		}
	case 0xC0, 0xC2:
		// TLS (or USB Type-C) identity: tag, 2-byte structure length,
		// then a TLS certificate chain of 3-byte length-prefixed
		// certificates. Skip to the first certificate.
		start = 9
		length = int(head[6])<<16 | int(head[7])<<8 | int(head[8])
	default:
		return nil, errBadCertificate
	}

	der := make([]byte, length)
	for read := 0; read < length; {
		chunk := length - read
		if chunk > readChunk {
			chunk = readChunk
		}
		n, err := chip.GetDataObject(oid, uint16(start+read), der[read:read+chunk])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, errShortObject
		}
		read += n
	}
	return x509.ParseCertificate(der)
}
