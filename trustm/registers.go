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
// three bits. A packet larger than one frame travels as a chain of
// first-intermediate...-last fragments. Names and values from
// ifx_i2c_transport_layer.c in the Infineon host library.
const (
	pctrChainMask         = 0x07
	pctrChainNone         = 0x00 // the packet fits in a single frame
	pctrChainFirst        = 0x01
	pctrChainIntermediate = 0x02
	pctrChainLast         = 0x04
	pctrChainError        = 0x07 // either side aborts a broken chain
)

// Command APDU codes. Bit 7 asks the chip to also clear its last error code
// register, mirroring the Infineon host library (optiga_cmd.c):
// https://github.com/Infineon/optiga-trust-m/blob/develop/optiga/cmd/optiga_cmd.c
const (
	CMD_GET_DATA_OBJECT  = 0x01 | 0x80
	CMD_SET_DATA_OBJECT  = 0x02 | 0x80
	CMD_GET_RANDOM       = 0x0C | 0x80
	CMD_CALC_HASH        = 0x30 | 0x80
	CMD_CALC_SIGN        = 0x31 | 0x80
	CMD_VERIFY_SIGN      = 0x32 | 0x80
	CMD_CALC_SSEC        = 0x33 | 0x80
	CMD_GEN_KEYPAIR      = 0x38 | 0x80
	CMD_OPEN_APPLICATION = 0x70 | 0x80

	paramReadData  = 0x00 // GetDataObject: read data (not metadata)
	paramWriteData = 0x00 // SetDataObject: plain write at an offset
	paramTRNG      = 0x00 // GetRandom: true random number generator
	paramInitApp   = 0x00 // OpenApplication: initialize a clean context
	paramSHA256    = 0xE2 // CalcHash: SHA-256 algorithm identifier
	paramECDSA     = 0x11 // CalcSign/VerifySign: ECDSA FIPS 186-3 w/o hash
	paramECDH      = 0x01 // CalcSSec: ECDH according to NIST SP 800-56A
)

// InData and OutData of the cryptographic commands hold TLVs: a one-byte
// tag, a 16-bit big-endian length, then the value. Tag values from
// optiga_cmd.c and the Solution Reference Manual command tables.
const (
	tagDigest    = 0x01 // CalcSign, VerifySign: the digest
	tagSignature = 0x02 // VerifySign: the signature over the digest
	tagSignKey   = 0x03 // CalcSign: OID of the signature key

	tagPrivateKey = 0x01 // GenKeyPair, CalcSSec: OID of the private key
	tagKeyUsage   = 0x02 // GenKeyPair: key usage identifier
	tagAlgorithm  = 0x05 // VerifySign, CalcSSec: curve of the public key
	tagPublicKey  = 0x06 // VerifySign, CalcSSec: external public key
	tagExport     = 0x07 // CalcSSec: export the shared secret

	tagDigestOut    = 0x01 // CalcHash response: the digest
	tagPublicKeyOut = 0x02 // GenKeyPair response: the public key
)

// CalcHash InData starts with a sequence tag telling the chip how this
// part of the message continues the running hash calculation
const (
	hashStart         = 0x00
	hashStartAndFinal = 0x01
	hashContinue      = 0x02
	hashFinal         = 0x03
)

// Curve selects an ECC curve, in the chip's algorithm identifier encoding
// (OPTIGA_ECC_CURVE_* in the Infineon host library)
type Curve uint8

const (
	P256 Curve = 0x03 // NIST P-256
	P384 Curve = 0x04 // NIST P-384
)

// KeyUsage restricts the operations a generated private key can be used
// for. Values can be combined with bitwise OR.
type KeyUsage uint8

const (
	KeyUsageAuth     KeyUsage = 0x01 // external authentication
	KeyUsageSign     KeyUsage = 0x10 // ECDSA signature calculation
	KeyUsageKeyAgree KeyUsage = 0x20 // ECDH key agreement
)

// Object identifiers of the ECC private key slots and the device
// certificate. The device key and its certificate are provisioned by
// Infineon; the user key slots are free for GenKeyPair. Private keys can
// never be read back out of the chip.
const (
	OID_DEVICE_KEY         = 0xE0F0
	OID_USER_KEY_1         = 0xE0F1
	OID_USER_KEY_2         = 0xE0F2
	OID_USER_KEY_3         = 0xE0F3
	OID_DEVICE_CERTIFICATE = 0xE0E0
)

// Data object identifier of the read-only coprocessor UID, and its size.
// See "Coprocessor UID OPTIGA Trust Family" in the Solution Reference Manual.
const (
	OID_COPROCESSOR_UID = 0xE0C2
	UIDLength           = 27
)

// Data object identifier of the last error code. When a command fails the
// chip stores the reason here; the code clears itself once read, and every
// command carrying the 0x80 bit clears it before executing.
const OID_LAST_ERROR_CODE = 0xF1C2

// Error codes reported in the last error code data object after a failed
// command, from the "Error codes" table in the Solution Reference Manual.
// The code of the most recent failure is available from
// Device.LastErrorCode.
const (
	ERR_INVALID_OID              uint8 = 0x01
	ERR_INVALID_PARAM_FIELD      uint8 = 0x03
	ERR_INVALID_LENGTH_FIELD     uint8 = 0x04
	ERR_INVALID_PARAM_IN_DATA    uint8 = 0x05
	ERR_INTERNAL_PROCESS         uint8 = 0x06
	ERR_ACCESS_CONDITIONS        uint8 = 0x07
	ERR_BOUNDARY_EXCEEDED        uint8 = 0x08
	ERR_METADATA_TRUNCATION      uint8 = 0x09
	ERR_INVALID_COMMAND_FIELD    uint8 = 0x0A
	ERR_COMMAND_OUT_OF_SEQUENCE  uint8 = 0x0B
	ERR_COMMAND_NOT_AVAILABLE    uint8 = 0x0C
	ERR_INSUFFICIENT_MEMORY      uint8 = 0x0D
	ERR_COUNTER_LIMIT_EXCEEDED   uint8 = 0x0E
	ERR_INVALID_MANIFEST         uint8 = 0x0F
	ERR_WRONG_PAYLOAD_VERSION    uint8 = 0x10
	ERR_INVALID_METADATA         uint8 = 0x11
	ERR_UNSUPPORTED_EXTENSION    uint8 = 0x24
	ERR_UNSUPPORTED_PARAMS       uint8 = 0x25
	ERR_INVALID_CERTIFICATE      uint8 = 0x29
	ERR_UNSUPPORTED_CERTIFICATE  uint8 = 0x2A
	ERR_SIGNATURE_VERIFY_FAILURE uint8 = 0x2C
	ERR_INTEGRITY_VIOLATED       uint8 = 0x2D
	ERR_DECRYPTION_FAILURE       uint8 = 0x2E
	ERR_AUTHORIZATION_FAILURE    uint8 = 0x2F
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
