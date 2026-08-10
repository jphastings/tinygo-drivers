package tsl2591

// I2CAddress is the only I2C address the TSL2591 answers on for read/write
// access. (Devices also answer a fixed, read-only 0x28 for a fast burst
// read of the data registers; this driver does not use it.)
const I2CAddress uint16 = 0x29

// deviceID is the fixed value the ID register (0x12) always reads back,
// regardless of gain/package variant.
const deviceID uint8 = 0x50

// Register addresses, see the "Register Description" section of the
// datasheet. These are the register field of a COMMAND byte, not bus
// addresses to write directly: see commandNormal.
const (
	regEnable  uint8 = 0x00
	regConfig  uint8 = 0x01
	regAILTL   uint8 = 0x04 // ALS interrupt low threshold, low byte
	regAILTH   uint8 = 0x05 // ALS interrupt low threshold, high byte
	regAIHTL   uint8 = 0x06 // ALS interrupt high threshold, low byte
	regAIHTH   uint8 = 0x07 // ALS interrupt high threshold, high byte
	regNPAILTL uint8 = 0x08 // No-persist ALS interrupt low threshold, low byte
	regNPAILTH uint8 = 0x09 // No-persist ALS interrupt low threshold, high byte
	regNPAIHTL uint8 = 0x0A // No-persist ALS interrupt high threshold, low byte
	regNPAIHTH uint8 = 0x0B // No-persist ALS interrupt high threshold, high byte
	regPersist uint8 = 0x0C // Interrupt persistence filter
	regPID     uint8 = 0x11 // Package ID
	regID      uint8 = 0x12 // Device ID
	regStatus  uint8 = 0x13 // Device status
	regC0DATAL uint8 = 0x14 // CH0 (full-spectrum) ADC, low byte
	regC0DATAH uint8 = 0x15 // CH0 (full-spectrum) ADC, high byte
	regC1DATAL uint8 = 0x16 // CH1 (infrared) ADC, low byte
	regC1DATAH uint8 = 0x17 // CH1 (infrared) ADC, high byte
)

// COMMAND byte construction. Every bus transaction starts with a COMMAND
// byte: bit 7 (CMD) must be set, bits 6:5 (TRANSACTION) select what the
// remaining bits 4:0 (ADDR/SF) mean:
//
//   - 0b01 ("normal operation"): ADDR/SF addresses one of the registers
//     above for the read or write that follows.
//   - 0b11 ("special function"): ADDR/SF instead selects one of a handful
//     of fixed commands, none of which address a register - used here only
//     to clear a pending interrupt.
//
// Confusing the two (e.g. always OR'ing register addresses with 0xE0, or
// special-function codes with 0xA0) silently talks to the wrong register or
// issues the wrong command, since I2C has no way to report the mistake.
const (
	commandNormal  uint8 = 0xA0 // CMD | TRANSACTION(01) | ADDR, ADDR OR'd in per access
	commandSpecial uint8 = 0xE0 // CMD | TRANSACTION(11) | SF, SF OR'd in per access
)

// Special-function codes, valid only OR'd with commandSpecial. The other
// two documented codes (force/clear a single no-persist interrupt) are not
// exposed by this driver, which does not otherwise manage interrupts.
const (
	sfClearALSAndNoPersistInterrupt uint8 = 0x07 // Clears ALS and no-persist ALS interrupt
)

// ENABLE register (0x00) bits. A plain, polled reading only needs
// PON and AEN; AIEN/NPIEN/SAI only matter if using the interrupt pin.
const (
	enablePON   uint8 = 1 << 0 // Power ON: starts the oscillator, timers and ADCs.
	enableAEN   uint8 = 1 << 1 // ALS Enable: starts ALS conversions.
	enableAIEN  uint8 = 1 << 4 // ALS Interrupt Enable.
	enableSAI   uint8 = 1 << 6 // Sleep After Interrupt: power down once an interrupt fires.
	enableNPIEN uint8 = 1 << 7 // No-Persist Interrupt Enable.
)

// STATUS register (0x13) bits.
const (
	statusAVALID uint8 = 1 << 0 // ALS Valid: a conversion has completed since AEN was asserted.
	statusAINT   uint8 = 1 << 4 // ALS Interrupt asserted.
	statusNPINTR uint8 = 1 << 5 // No-persist interrupt asserted.
)
