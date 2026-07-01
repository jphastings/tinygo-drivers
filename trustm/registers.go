package trustm

// Default 7-bit I2C address of the OPTIGA Trust M
const Address = 0x30

// Register addresses of the Infineon I2C protocol physical layer. Names and
// values taken from the Infineon host library:
// https://github.com/Infineon/optiga-trust-m/blob/develop/optiga/comms/ifx_i2c/ifx_i2c_physical_layer.c
const (
	REG_DATA          = 0x80 // writing starts a frame, reading fetches one
	REG_DATA_REG_LEN  = 0x81 // maximum frame size, 2 bytes big endian
	REG_I2C_STATE     = 0x82 // 4 bytes: flags, then pending frame length
	REG_BASE_ADDR     = 0x83 // I2C address reconfiguration (unused here)
	REG_MAX_SCL_FREQU = 0x84 // bus speed negotiation (unused here)
	REG_SOFT_RESET    = 0x88 // writing 2 bytes triggers a warm reset
	REG_I2C_MODE      = 0x89 // bus mode negotiation (unused here)
)

// Flag bits of I2C_STATE byte 0
const (
	stateResponseReady = 0x40 // a frame is waiting to be read from DATA
	stateSoftReset     = 0x08 // the SOFT_RESET register is supported
)

// Data link layer frame control (FCTR) fields. A frame is FCTR, a 16-bit
// big-endian payload length, the payload, and a 16-bit big-endian CRC.
const (
	fctrControlFrame = 0x80 // set for control (ACK/NACK) frames

	fctrSeqctrPos  = 5    // bits 6:5, acknowledgement type
	fctrSeqctrMask = 0x60 // masks seqctrAck, seqctrNack or seqctrResync
	seqctrAck      = 0
	seqctrNack     = 1
	seqctrResync   = 2

	fctrFrameNrPos  = 2    // bits 3:2, sequence number of this frame
	fctrFrameNrMask = 0x0C //
	fctrAckNrMask   = 0x03 // bits 1:0, sequence number being acknowledged
)

// Both sides start their 2-bit frame sequence counter at 3, so that the
// first data frame sent carries sequence number 0
const seqInit = 3

// Transport layer packet control byte (PCTR): chaining status in the low
// three bits. This driver only supports unchained packets.
const pctrChainNone = 0x00

// Command APDU codes. Bit 7 asks the chip to also clear its last error code
// register, mirroring the Infineon host library (optiga_cmd.c):
// https://github.com/Infineon/optiga-trust-m/blob/develop/optiga/cmd/optiga_cmd.c
const (
	CMD_GET_DATA_OBJECT  = 0x01 | 0x80
	CMD_GET_RANDOM       = 0x0C | 0x80
	CMD_OPEN_APPLICATION = 0x70 | 0x80

	paramReadData = 0x00 // GetDataObject: read data (not metadata)
	paramTRNG     = 0x00 // GetRandom: true random number generator
	paramInitApp  = 0x00 // OpenApplication: initialize a clean context
)

// Data object identifier of the read-only coprocessor UID, and its size.
// See "Coprocessor UID OPTIGA Trust Family" in the Solution Reference Manual.
const (
	OID_COPROCESSOR_UID = 0xE0C2
	UIDLength           = 27
)

// GetRandom accepts requests for 8 to 256 bytes of randomness
const (
	minRandomLength = 8
	maxRandomLength = 256
)

// The unique application identifier sent with OpenApplication
// ("\xd2v\x00\x00\x04GenAuthAppl", from optiga_cmd.c)
var applicationID = [16]byte{
	0xD2, 0x76, 0x00, 0x00, 0x04, 0x47, 0x65, 0x6E,
	0x41, 0x75, 0x74, 0x68, 0x41, 0x70, 0x70, 0x6C,
}
