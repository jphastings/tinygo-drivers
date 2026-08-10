# TLS integration brief

How to build TLS (1.3) on top of this driver. Written as a handoff to whoever implements that layer — a general-purpose package that should live in its own project, not in tinygo-drivers, and consume this driver only through the public API described here. It should serve chip-backed TLS **clients** (device identity) and **servers** (CA-issued identity) alike.

The design target is TinyGo on an **ESP32-C3**, with WiFi on the same device from [espradio](https://github.com/tinygo-org/espradio). espradio implements TinyGo's netdev/netlink interface, so the stdlib `net` package works on-device — the TLS layer should therefore be written against plain `net.Conn`/`net.Listener` and know nothing about espradio itself. That keeps it portable to any target where TLS runs as Go code on the MCU (and to big-Go hosts, which is also how to test it against real peers).

I'd suggest supporting only TLS 1.3. It removes most of what makes a 1.2 implementation large (renegotiation, CBC modes, RSA key exchange) and everything a new embedded deployment talks to speaks it.

**Precondition**: VALIDATION.md must be green on real silicon first — the sign/verify and chaining stages especially, since every handshake will exercise them.

## What the chip contributes to a TLS handshake

| Handshake need              | Driver API                                                                                                            |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Identity certificate        | `signer.DeviceCertificate` (factory, OID 0xE0E0) or a CA-issued cert for a generated key — see "Two identity stories" |
| CertificateVerify signature | `signer.Signer` (crypto.Signer over `CalcSign`)                                                                       |
| Ephemeral ECDHE key share   | host X25519 recommended; chip `GenKeyPair` + `ECDH` (P-256/384) as an alternative — see "Key exchange"                |
| Randomness                  | `trustm.GetRandom` (AIS-31 TRNG)                                                                                      |
| Transcript hashing          | host `crypto/sha256` (chip `CalcHash` works but is slower over I2C)                                                   |

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

This snippet shows the *shape* of the seam using big-Go's types; on TinyGo
targets `crypto/tls` is a stub, so the package will define its own
config/certificate types — the point is that they should accept any
`crypto.Signer`, nothing chip-specific.

`Signer.Sign` returns standard ASN.1 signatures (the chip's bare r,s
encoding is converted internally), so `crypto/tls`, `x509` and
`ecdsa.VerifyASN1` all accept them directly. The chip signs externally
supplied digests (`paramECDSA` is "ECDSA without hash"), which is exactly
what CertificateVerify needs — in TLS 1.3 client and server sign the same
construction, so one code path serves both roles. Signature algorithms:
`ecdsa_secp256r1_sha256` (and `ecdsa_secp384r1_sha384`); the chip supports
no other TLS-relevant keys.

## Two identity stories

- **Factory device identity** (typical for clients / device attestation):
  the pre-provisioned key in `OID_DEVICE_KEY` and its certificate from
  `signer.DeviceCertificate` (0xE0E0), chaining to Infineon's CA — the
  peer must be configured to trust that chain.
- **CA-issued identity** (typical for servers, e.g. ACME/Let's Encrypt):
  `signer.Generate` into `OID_USER_KEY_1`–`3` (a one-time NVM write is
  fine for a long-lived identity key — the endurance concern below is
  about *per-connection* use), build a CSR with
  `x509.CreateCertificateRequest` signed by the same `crypto.Signer`,
  and store the CA-issued chain in flash or the writable certificate
  slots 0xE0E1–0xE0E3. The chain arrives from the CA, so nothing needs
  to be parsed out of chip cert objects on this path.

## Portability caveat: targets where TLS cannot run in Go

On several TinyGo network targets TLS is **offloaded to WiFi co-processor
firmware** (NINA, ESP-AT), which cannot call back into a crypto.Signer —
chip-backed certificates are impossible there, and this package cannot
support those targets. It applies only where the TLS implementation is Go
code on the MCU (espradio-class targets) or a big-Go host.

## Key exchange: default to host X25519

The chip only speaks P-256/P-384, but TLS 1.3 peers — Go clients and
servers, browsers, ACME validators — lead with an X25519 key share. An
endpoint that only offers P-256 forces a HelloRetryRequest round trip on
nearly every handshake with such peers. Software X25519
(`crypto/ecdh` X25519, or `golang.org/x/crypto/curve25519`) is small,
constant-time, and free of `math/big`.

So the recommended split is: **host ephemerals (X25519) + chip
CertificateVerify**. The chip's non-extractable identity key is the part
that matters; ephemerals are secret only for the connection's lifetime.
This split also sidesteps both caveats of chip-side ECDHE below, and
reduces the chip's per-handshake work to one `CalcSign` (plus
`GetRandom`), which softens the serialization constraint under
concurrent handshakes.

Chip-side P-256 ECDHE (`GenKeyPair` + `ECDH`) remains a legitimate
alternative where peers negotiate P-256 or policy demands chip-held
ephemerals. Its caveats:

- `GenKeyPair` into 0xE0F1–0xE0F3 writes NVM — wrong for per-connection
  ephemerals (endurance). Use the volatile **session contexts
  0xE100–0xE103** instead: generating a P-256 key-agreement key into
  0xE100 and running `ECDH` against a host peer key has been confirmed
  on silicon, and the shared secret matched Go's `crypto/ecdh` exactly.
  So chip-held ephemerals cost no NVM endurance at all.
- Without the shielded connection the ECDH secret crosses the I2C bus
  in plaintext (see SHIELDED_CONNECTION.md). If bus probing is in the
  threat model, implement shielded first — or stay with host ephemerals.

## Host-side crypto and RAM on the ESP32-C3

The host share of the handshake (record encryption, HKDF, transcript
hashing) runs in software on a single 160MHz RISC-V core; TinyGo has no
driver for the C3's AES/SHA accelerators (yet).

- **Cipher suites**: prefer `TLS_CHACHA20_POLY1305_SHA256` — ChaCha20 is
  designed to be fast in software, and constant-time software AES is not.
  RFC 8446 makes `TLS_AES_128_GCM_SHA256` the mandatory suite, so
  implement it for peers that insist, but expect it to be the slow path
  until an accelerator driver exists.
- **RAM**: the C3 has ~400kB of SRAM shared with espradio, the network
  stack and the application. Per-connection record buffers dominate a TLS
  implementation's footprint — peers may send 16KiB records — so support
  the `record_size_limit` extension (RFC 8449) to negotiate them down,
  and make buffer sizes configurable.

## Implementation notes

- **Scheduling**: all of the driver's waits (response polling, wake-up
  retries, guard time) are `time.Sleep`, which yields TinyGo's
  cooperative scheduler — a slow chip operation blocks its goroutine,
  not the radio or the network stack.
- **Performance**: budget one chip round trip per private-key operation;
  see the SRM "Command performance" section for expected latencies, and
  measure per VALIDATION.md before promising handshake times. Everything
  is synchronous and the Device is not safe for concurrent use — serialize
  all chip access behind one lock in the TLS layer. With host ephemerals
  only CertificateVerify contends, so concurrent handshakes simply queue
  at the signer.
- **Errors**: chip refusals surface as mapped errors plus
  `Device.LastErrorCode()` (ERR_* in registers.go) for diagnosis.

## Known gaps that would fall on the TLS layer

- Session-context key generation is unvalidated (chip-ECDHE path only).
- No shielded connection yet (chip-ECDHE path / bus-probing threat model
  only; see SHIELDED_CONNECTION.md).
- `signer.Certificate` returns only the first certificate of a 0xE0E0
  chain. Relevant only when presenting the factory chain with
  intermediates; CA-issued chains live outside the chip and don't hit
  this.
