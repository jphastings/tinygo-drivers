# Shielded connection: design notes

The shielded connection encrypts and authenticates the I2C link between
host and chip, so commands, data objects and (crucially) exported ECDH
secrets cannot be read or tampered with by probing the bus. It is
**deliberately not implemented yet**: it is a cryptographic handshake
whose mistakes are silently insecure, and it cannot be meaningfully
validated against a fake chip. Do this only after VALIDATION.md is green
on real silicon.

## How it works (verified against SRM §6.6 and the Infineon host library)

- A 64-byte pre-shared **platform binding secret** lives in data object
  0xE140 (SRM Table 73) and mirrored on the host. Pairing happens once at
  first boot; ≥32 bytes of entropy recommended (SRM §6.5.8).
- A **presentation layer** slots between our transport layer and command
  layer (`ifx_i2c_presentation_layer.c`). Its record starts with an SCTR
  byte: protocol in the upper bits (handshake 0x00, record exchange 0x20,
  alert 0x40) and message type in the lower bits (hello 0x00,
  finished 0x08).
- **Handshake**: hello messages exchange a protocol version and 32-byte
  randoms; a key block is derived from the binding secret with the
  TLS 1.2 PRF (SHA-256) — the C host calls `pal_crypt_tls_prf_sha256` —
  and finished messages prove both sides derived the same keys.
- **Record protection**: AES-128-CCM. The 40-byte key block splits into
  encrypt/decrypt keys (16 bytes each, offsets 0x00/0x10) and 4-byte
  implicit nonces (offsets 0x20/0x24), combined with per-record sequence
  numbers. Alerts (fatal / integrity-violated) abort the session.
- The PCTR presence bit (0x08) marks frames that carry presentation-layer
  records; our code currently always sends it cleared.

**Transcribe before implementing** (not yet byte-verified): the exact PRF
label and seed layout, the CCM nonce construction and MAC length, and the
finished-message contents — all in `ifx_i2c_presentation_layer.c` and
`pal_crypt_mbedtls.c`.

## Implementation sketch

- New `shielded.go`: handshake state machine and record encrypt/decrypt,
  hooked into `transceive` where the packet meets the PCTR byte.
- Go's stdlib has no CCM mode: implement AES-CCM over crypto/aes
  (small; test against RFC 3610 vectors). Same for the TLS 1.2 PRF
  (RFC 5246 vectors).
- API sketch: `Configure` unchanged; `EnableShielded(secret []byte)`
  performs pairing/handshake; per-command protection levels (the C host's
  command/response/full distinction) can come later — start with
  everything-protected, it is simpler to reason about.
- Device RAM cost: one extra frame-sized buffer for record
  encryption plus key material (~100 bytes) — acceptable next to the
  existing staging buffers.

## Testing strategy

1. Unit-test PRF and CCM against public RFC vectors (no chip needed).
2. First hardware session: pair, handshake, then a shielded `GetRandom`.
3. Capture one full handshake with a logic analyzer while the *C host
   library* drives the chip; replay those frames as golden data in the
   fake-chip tests so regressions are catchable offline afterwards.

## Cautions

- Writing 0xE140 and shielded handshakes raise **security events**: the
  chip's security monitor throttles rapid repetitions (SRM §4.6) — do not
  loop handshakes in tests; reuse sessions.
- Pairing writes NVM; the binding secret object has limited endurance
  (SRM §6.5.8). Pair once, not per boot.
- Losing the host copy of the binding secret while the chip's copy has
  access conditions locked down bricks shielded mode — decide the
  provisioning/recovery story before locking anything.
