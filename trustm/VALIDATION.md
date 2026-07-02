# Hardware validation plan

This driver was written and reviewed against the Infineon host library and
the Solution Reference Manual (SRM), with all protocol behaviour tested
against a fake chip — it has **not yet run against real silicon**. This is
the bring-up plan for when a board (e.g. Adafruit 4351) is available.

Work through the stages in order: each one depends on the layers the
previous stage proved, so the first failure cleanly identifies the broken
layer.

## Stages

1. **`Connected()`** — proves I2C addressing (0x30) and register reads
   (I2C_STATE). Failure here is wiring, pull-ups, or bus speed, not
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
6. **`SetDataObject` of >266 bytes** into an arbitrary data object
   (0xF1D0, type 2, per SRM Table 79) — first exercise of **outbound
   chaining**. This is the top-risk item: the assumption that the chip
   ACKs every non-final fragment with a control frame before accepting
   the next came from the C state machines, not from observation. Read
   the object back and compare.
7. **`CalcHash` of >693 bytes** — exercises the start/continue/final
   hash sequencing across commands. Compare against a host-computed
   SHA-256 of the same input.
8. **Sign/verify round trip** — GenKeyPair, CalcSign, VerifySign; then
   corrupt the digest and confirm `ErrSignatureInvalid` and
   `LastErrorCode() == ERR_SIGNATURE_VERIFY_FAILURE` (0x2C). This also
   validates the last-error-code fetch (the non-clearing GetDataObject).
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

`guardTime` (50µs), `wakeInterval`/`wakeRetries` (1ms × 200),
`pollInterval`/`pollRetries` (5ms × 1000), `resetStartup` (15ms). All
taken from the Infineon host library configuration; adjust only with a
measured reason.

## Debugging tips

- A logic analyzer on SDA/SCL plus the constants above makes frames easy
  to read: `[addr] 0x80 FCTR LEN₂ PCTR APDU… CRC₂` for writes, register
  0x82 polls in between.
- NACKed address during wake-up and guard-time gaps after every
  transaction are protocol behaviour, not errors.
- If Configure fails after reset on a board that previously worked, the
  data-link counters may be desynced from an aborted session: power-cycle
  the board (soft reset normally covers this, but it is unvalidated).
