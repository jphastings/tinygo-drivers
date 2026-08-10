package signer

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"

	"tinygo.org/x/drivers/trustm"
)

// fakeChip implements Chip with a real software ECDSA key, so signatures
// and keys produced through the adapter can be checked against Go's own
// crypto as an independent oracle.
type fakeChip struct {
	key *ecdsa.PrivateKey
	obj []byte // contents of the certificate data object
}

func (f *fakeChip) CalcSign(keyOID uint16, digest, sig []byte) (int, error) {
	size := (f.key.Curve.Params().BitSize + 7) / 8
	// The chip refuses a digest longer than the key (SRM Table 51)
	if len(digest) > size {
		return 0, errors.New("digest longer than the signature key")
	}
	r, s, err := ecdsa.Sign(rand.Reader, f.key, digest)
	if err != nil {
		return 0, err
	}
	return copy(sig, chipSignature(r, s, size)), nil
}

func (f *fakeChip) GenKeyPair(keyOID uint16, curve trustm.Curve, usage trustm.KeyUsage, pub []byte) (int, error) {
	c, err := ellipticCurve(curve)
	if err != nil {
		return 0, err
	}
	f.key, err = ecdsa.GenerateKey(c, rand.Reader)
	if err != nil {
		return 0, err
	}
	chipKey, err := MarshalPublicKey(&f.key.PublicKey)
	if err != nil {
		return 0, err
	}
	return copy(pub, chipKey), nil
}

func (f *fakeChip) GetDataObject(oid uint16, offset uint16, data []byte) (int, error) {
	if int(offset) >= len(f.obj) {
		return 0, nil
	}
	n := copy(data, f.obj[offset:])
	// Return less than asked for, so readers must loop
	if n > 100 {
		n = 100
	}
	return n, nil
}

// chipSignature encodes r and s the way the chip does: two DER INTEGERs with
// no outer SEQUENCE, each holding the full key size behind a leading zero
// byte. That zero is redundant whenever the value's top bit is clear, which
// is why Infineon's host library re-checks the padding of every signature it
// reads back (optiga_cmd_ecc_r_s_padding_check).
func chipSignature(r, s *big.Int, size int) []byte {
	out := []byte{}
	for _, v := range []*big.Int{r, s} {
		b := make([]byte, size+1)
		v.FillBytes(b[1:])
		out = append(out, 0x02, byte(len(b)))
		out = append(out, b...)
	}
	return out
}

// TestSignVerifiesWithGoCrypto generates a key through the adapter, signs a
// digest, and requires Go's own ECDSA verification to accept the signature
func TestSignVerifiesWithGoCrypto(t *testing.T) {
	for _, curve := range []trustm.Curve{trustm.P256, trustm.P384} {
		chip := &fakeChip{}
		s, err := Generate(chip, trustm.OID_USER_KEY_1, curve)
		if err != nil {
			t.Fatal(err)
		}

		pub, ok := s.Public().(*ecdsa.PublicKey)
		if !ok || !pub.Equal(&chip.key.PublicKey) {
			t.Fatalf("curve %#02x: Public() does not match the generated key", curve)
		}

		digest := sha256.Sum256([]byte("tinygo"))
		sig, err := s.Sign(nil, digest[:], nil)
		if err != nil {
			t.Fatal(err)
		}
		if !ecdsa.VerifyASN1(pub, digest[:], sig) {
			t.Errorf("curve %#02x: Go crypto rejects the converted signature", curve)
		}
	}
}

// TestASN1SignaturePinsEncoding pins the conversion from the chip's bare r,s
// encoding to standard ASN.1: the SEQUENCE is added, a leading zero the value
// does not need is dropped, and one it does need is kept
func TestASN1SignaturePinsEncoding(t *testing.T) {
	for name, tc := range map[string]struct{ chip, want []byte }{
		"redundant zero dropped from r and s": {
			chip: []byte{0x02, 0x03, 0x00, 0x11, 0x22, 0x02, 0x03, 0x00, 0x33, 0x44},
			want: []byte{0x30, 0x08, 0x02, 0x02, 0x11, 0x22, 0x02, 0x02, 0x33, 0x44},
		},
		"zero kept where the top bit is set": {
			chip: []byte{0x02, 0x03, 0x00, 0x91, 0x22, 0x02, 0x03, 0x00, 0x33, 0x44},
			want: []byte{0x30, 0x09, 0x02, 0x03, 0x00, 0x91, 0x22, 0x02, 0x02, 0x33, 0x44},
		},
		"already minimal is only wrapped": {
			chip: []byte{0x02, 0x02, 0x11, 0x22, 0x02, 0x02, 0x33, 0x44},
			want: []byte{0x30, 0x08, 0x02, 0x02, 0x11, 0x22, 0x02, 0x02, 0x33, 0x44},
		},
	} {
		got, err := asn1Signature(tc.chip)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%s: asn1Signature(%X) = %X, want %X", name, tc.chip, got, tc.want)
		}
	}

	for name, chip := range map[string][]byte{
		"truncated":        {0x02, 0x04, 0x11, 0x22},
		"not an integer":   {0x04, 0x02, 0x11, 0x22, 0x02, 0x02, 0x33, 0x44},
		"trailing garbage": {0x02, 0x02, 0x11, 0x22, 0x02, 0x02, 0x33, 0x44, 0x00},
		"only r":           {0x02, 0x02, 0x11, 0x22},
	} {
		if _, err := asn1Signature(chip); err != errBadSignature {
			t.Errorf("%s: asn1Signature(%X) = %v, want errBadSignature", name, chip, err)
		}
	}
}

// TestSignTruncatesOversizedDigest checks the crypto.Signer contract against
// the chip's refusal to sign a digest longer than the key: a SHA-384 digest
// handed to a P-256 key must be cut to its leftmost 32 bytes, as ECDSA says
func TestSignTruncatesOversizedDigest(t *testing.T) {
	chip := &fakeChip{}
	s, err := Generate(chip, trustm.OID_USER_KEY_1, trustm.P256)
	if err != nil {
		t.Fatal(err)
	}

	digest := sha512.Sum384([]byte("tinygo"))
	sig, err := s.Sign(nil, digest[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ecdsa.VerifyASN1(&chip.key.PublicKey, digest[:32], sig) {
		t.Error("signature does not verify over the truncated digest")
	}
}

func TestPublicKeyRoundTrip(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	chipKey, err := MarshalPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(chipKey) != 68 || chipKey[0] != 0x03 || chipKey[1] != 0x42 ||
		chipKey[2] != 0x00 || chipKey[3] != 0x04 {
		t.Errorf("marshaled key starts %X, want the 68 byte BIT STRING 03 42 00 04 ...", chipKey[:4])
	}

	pub, err := ParsePublicKey(trustm.P256, chipKey)
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equal(&key.PublicKey) {
		t.Error("parsed key does not match the original")
	}

	// A key that is not on the curve must be rejected
	chipKey[10] ^= 0x01
	if _, err := ParsePublicKey(trustm.P256, chipKey); err == nil {
		t.Error("expected an off-curve key to be rejected")
	}
}

func selfSignedDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "OPTIGA test device"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestDeviceCertificate(t *testing.T) {
	der := selfSignedDER(t)

	// TLS identity framing: tag C0 and a 2-byte structure length, then the
	// chain and first certificate with 3-byte lengths
	wrapped := []byte{
		0xC0, byte((len(der) + 6) >> 8), byte(len(der) + 6),
		0x00, byte((len(der) + 3) >> 8), byte(len(der) + 3),
		0x00, byte(len(der) >> 8), byte(len(der)),
	}
	wrapped = append(wrapped, der...)

	for name, obj := range map[string][]byte{"plain DER": der, "TLS identity": wrapped} {
		chip := &fakeChip{obj: obj}
		cert, err := DeviceCertificate(chip)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cert.Subject.CommonName != "OPTIGA test device" {
			t.Errorf("%s: parsed subject %q", name, cert.Subject.CommonName)
		}
		if !bytes.Equal(cert.Raw, der) {
			t.Errorf("%s: reassembled DER does not match", name)
		}
	}

	chip := &fakeChip{obj: []byte{0x55, 0, 0, 0, 0, 0, 0, 0, 0}}
	if _, err := DeviceCertificate(chip); err != errBadCertificate {
		t.Errorf("unknown framing: %v, want errBadCertificate", err)
	}
}
