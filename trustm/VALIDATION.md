# Hardware validation

This driver was written and reviewed against the Infineon host library and
the Solution Reference Manual (SRM), with all protocol behaviour tested
against a fake chip. **Every stage below has since been run against real
silicon**, over an MCP2221A, and passed.

Work through the stages in order: each one depends on the layers the
previous stage proved, so the first failure cleanly identifies the broken
layer.

## What silicon has shown

Run against an Infineon-provisioned part (firmware identifier 80101071,
`CN=Infineon IoT Node`):

- **Transport works end to end.** `Configure()` completes soft reset,
  frame-size negotiation and the OpenApplication round trip, which
  exercises data-link framing, the CRC, sequence counters, ACKs and APDU
  coding in one call. DATA_REG_LEN reads 0x0115 (277 bytes).
- **The Coprocessor UID decodes per SRM Table 76**, beginning CD 16 33 for
  the CIM, platform and model identifiers.
- **Inbound chaining works.** The device certificate is larger than one
  frame and reads back intact, parsing as P-256 / ECDSA-SHA256.
- **The 0xC0 framing assumption was right.** 0xE0E0 begins
  `C0 01 E2 00 01 DF ...`, so the factory certificate does carry the TLS
  identity wrapper rather than plain DER.
- **Signing round-trips through Go and through the chip.** `crypto/ecdsa`
  accepts a chip signature against the certificate's public key, the chip's
  own `VerifySign` accepts it, and both reject a corrupted digest — the
  chip reporting `ErrSignatureInvalid`.
- **Signature padding**: the one signature captured needed no stripping
  (both INTEGERs were minimal already). The encoding path still has to
  handle the padded case, so how often it appears is unmeasured; capture
  more raw `CalcSign` output before concluding it is rare.
- **The sleep/wake behaviour is as documented.** The chip NACKs the first
  access after roughly 20ms of bus inactivity, and `wakeRetries` carries
  the driver through it. Anything bypassing the driver's `tx()` has to
  repeat that retry or it will see spurious NACKs.

- **Outbound chaining works** — the top-risk item, since the rule that the
  chip ACKs every non-final fragment before accepting the next came from
  Infineon's state machine rather than observation. A 600-byte write to
  0xF1E0, well past the 277-byte frame size, reads back byte-identical.
- **CalcHash sequences correctly across commands.** A 2000-byte message
  hashed on the chip matches the host's SHA-256 of the same input.
- **ECDH agrees with `crypto/ecdh`**, and the key can be **volatile**:
  generating a P-256 key-agreement key into session context 0xE100
  succeeded, and the resulting shared secret matched the host's exactly.
  Chip-held ephemerals therefore cost no NVM endurance, which TLS.md had
  flagged as an open question.

Only the chaining stage writes NVM, and only into an arbitrary data
object. Nothing here needs a key slot, metadata write, or lifecycle
transition — and the factory key in 0xE0F0 should be left alone whatever
else is tested, since overwriting it strands the device certificate that
verifies its signatures.

## Stages

1. **`Connected()`** — proves I2C addressing (0x30) and register reads
   (DATA_REG_LEN, whose value must be at least 0x0010). Failure here is
   wiring, pull-ups, bus speed, or another part answering at 0x30 — not
   protocol.
2. **`Configure()`** — one call exercises soft reset, frame-size
   negotiation (DATA_REG_LEN) and a full OpenApplication round trip:
   data-link framing, CRC, sequence counters, ACKs, APDU coding. This is
   the single most information-dense smoke test.
3. **`UID()`** — small unchained GetDataObject. Check the first bytes
   decode per the SRM "Coprocessor UID" table (CIM identifier etc.).
4. **`GetRandom`** — sanity-check output (no all-zero / repeated buffers
   across calls).
5. **Device certificate read** (`signer.DeviceCertificate`) — first
   exercise of **inbound chaining**: the factory certificate is larger
   than one frame. Also confirms the 0xC0 TLS-identity framing assumption.
6. **`SetDataObject` of >266 bytes** into an arbitrary data object —
   first exercise of **outbound chaining**. Use 0xF1E0 or 0xF1E1: those
   are the type 2 objects (1500 bytes, SRM Table 79). The 0xF1D0-0xF1DB
   range is type 3 and holds only 140 bytes, so a chained write there
   fails with ERR_BOUNDARY_EXCEEDED before the chaining is even tested.
   This is the top-risk item: the assumption that the chip ACKs every
   non-final fragment with a control frame before accepting the next came
   from the C state machines, not from observation. Read the object back
   and compare.
7. **`CalcHash` of >693 bytes** — exercises the start/continue/final
   hash sequencing across commands. Compare against a host-computed
   SHA-256 of the same input.
8. **Sign/verify round trip** — GenKeyPair, CalcSign, VerifySign; then
   corrupt the digest and confirm `ErrSignatureInvalid` and
   `LastErrorCode() == ERR_SIGNATURE_VERIFY_FAILURE` (0x2C). This also
   validates the last-error-code fetch (the non-clearing GetDataObject).
   Record the raw `CalcSign` output of several signatures: the chip is
   documented to return r and s as DER INTEGERs, but is known to pad them
   with a leading zero their value does not need, which
   `signer.Sign` has to strip before Go's ASN.1 readers will accept the
   signature. Confirm how often that padding actually appears.
9. **`ECDH`** — cross-check the shared secret against Go's crypto/ecdh
   with a host-generated peer key.
10. **`examples/trustm/signer`** — end-to-end: certificate parse plus a
    chip signature verified by Go's crypto/ecdsa.

## What to record

- Latency of OpenApplication, CalcSign, GenKeyPair and a chained
  certificate read (the SRM "Command performance" section has Infineon's
  expected figures to compare against).
- How often the wake-retry path triggers: the chip sleeps ~20 ms after
  bus inactivity (SRM "Sleep mode"), so NACK-then-retry on first access
  is *expected*, not a fault.
- Whether any non-final response fragment arrives smaller than the frame
  size. The driver treats that as `errBrokenChain` (the C host does too);
  if real chips do this, the receive path needs loosening.

## Tuning knobs (trustm.go constants)

`guardTime` (50µs), `wakeInterval`/`wakeRetries` (1ms × 200) and
`pollInterval` (5ms) are `PL_GUARD_TIME_INTERVAL_US`,
`PL_POLLING_INVERVAL_US`/`PL_POLLING_MAX_CNT` and
`PL_DATA_POLLING_INVERVAL_US` from the Infineon host library's
`ifx_i2c_config.h`. `pollRetries` (1000, so a 5s ceiling) is this
driver's own choice, far below the C host's 180s
`TL_MAX_EXIT_TIMEOUT`. `resetStartup` (15ms) comes from the datasheet's
start-up guarantee; the C host waits 12ms here — its
`STARTUP_TIME_MSEC` is 12000, but the callback it feeds takes
*microseconds*, despite the constant's name. Adjust only with a measured
reason.

## Debugging tips

- A logic analyzer on SDA/SCL plus the constants above makes frames easy
  to read: `[addr] 0x80 FCTR LEN₂ PCTR APDU… CRC₂` for writes, register
  0x82 polls in between.
- NACKed address during wake-up and guard-time gaps after every
  transaction are protocol behaviour, not errors.
- If Configure fails after reset on a board that previously worked, the
  data-link counters may be desynced from an aborted session: power-cycle
  the board (soft reset normally covers this, but it is unvalidated).
