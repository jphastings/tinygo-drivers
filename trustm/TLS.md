# TLS integration brief

How to build TLS (1.3 or 1.2) on top of this driver. Written as a handoff
to whoever implements that layer — it should live in its own project, not
in tinygo-drivers, and consume this driver only through the public API
described here.

## What the chip contributes to a TLS handshake

| Handshake need              | Driver API                                         |
|-----------------------------|----------------------------------------------------|
| Client identity certificate | `signer.DeviceCertificate` (OID 0xE0E0)            |
| CertificateVerify signature | `signer.Signer` (crypto.Signer over `CalcSign`)    |
| Ephemeral ECDHE key share   | `trustm.GenKeyPair` + `trustm.ECDH`                |
| Randomness                  | `trustm.GetRandom` (AIS-31 TRNG)                   |
| Transcript hashing          | host `crypto/sha256` (chip `CalcHash` works but is slower over I2C) |

Everything else — HKDF/PRF, record encryption, the state machine — runs on
the host; the chip offers no acceleration there.

## The intended seam: crypto.Signer

`trustm/signer` exists precisely so a TLS layer never touches chip
internals:

```go
cert, _ := signer.DeviceCertificate(&chip)
s := signer.New(&chip, trustm.OID_DEVICE_KEY, cert.PublicKey.(*ecdsa.PublicKey))
tlsCert := tls.Certificate{
        Certificate: [][]byte{cert.Raw},
        PrivateKey:  s, // any crypto.Signer works here
        Leaf:        cert,
}
```

`Signer.Sign` returns standard ASN.1 signatures (the chip's bare r,s
encoding is converted internally), so `crypto/tls`, `x509` and
`ecdsa.VerifyASN1` all accept them directly.

## Check first: where does TLS actually run on the target?

On several TinyGo network targets TLS is **offloaded to WiFi co-processor
firmware** (NINA, ESP-AT), which cannot call back into a crypto.Signer —
chip-backed client certificates are impossible there. This brief only
applies where the TLS implementation is Go code on the MCU (or you are
writing one). Verify this before designing anything.

## Notes for a hand-rolled TLS 1.3 client

- **ECDHE**: generate the ephemeral in the chip (`GenKeyPair`, P-256) and
  derive with `ECDH`. Caveats:
  - `GenKeyPair` into 0xE0F1–0xE0F3 writes NVM — wrong for per-connection
    ephemerals (endurance). The chip has volatile **session contexts
    0xE100–0xE103** for exactly this; the driver passes any OID through,
    but session-context keys are unvalidated — test on silicon, and if
    they work, use them.
  - Without the shielded connection the ECDH secret crosses the I2C bus
    in plaintext (see SHIELDED_CONNECTION.md). If bus probing is in the
    threat model, either implement shielded first or do ECDHE on the host
    (host ECDHE + chip CertificateVerify is a coherent, common split —
    the chip's non-extractable identity key is the part that matters).
- **Key/curve support**: P-256 and P-384 only. Signature algorithm
  `ecdsa_secp256r1_sha256`; the chip signs externally supplied digests
  (`paramECDSA` is "ECDSA without hash"), which is exactly what
  CertificateVerify needs.
- **Performance**: budget one chip round trip per private-key operation;
  see the SRM "Command performance" section for expected latencies, and
  measure per VALIDATION.md before promising handshake times. Everything
  is synchronous and the Device is not safe for concurrent use — one
  handshake at a time, or add locking in the TLS layer.
- **Errors**: chip refusals surface as mapped errors plus
  `Device.LastErrorCode()` (ERR_* in registers.go) for diagnosis.

## Known gaps that would fall on the TLS layer

- Session-context key generation is unvalidated (above).
- No shielded connection yet (see SHIELDED_CONNECTION.md).
- `tls.Certificate` chains longer than the leaf: 0xE0E0 may hold a chain
  in the TLS-identity framing; `signer.Certificate` currently returns
  only the first certificate. Extend it if the server needs intermediates.
