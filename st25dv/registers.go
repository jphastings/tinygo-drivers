package st25dv

// I2C 7-bit device addresses. The ST25DV answers on two addresses: one for
// the user memory, dynamic registers and mailbox, and one for the system
// configuration area.
const (
	AddressUser   uint8 = 0x53
	AddressSystem uint8 = 0x57
)

// System configuration registers, accessible via AddressSystem. Names taken
// from the datasheet:
// https://www.st.com/resource/en/datasheet/st25dv16k.pdf
const (
	REG_GPO          uint16 = 0x0000
	REG_IT_TIME      uint16 = 0x0001
	REG_EH_MODE      uint16 = 0x0002
	REG_RF_MNGT      uint16 = 0x0003
	REG_RFA1SS       uint16 = 0x0004
	REG_ENDA1        uint16 = 0x0005
	REG_RFA2SS       uint16 = 0x0006
	REG_ENDA2        uint16 = 0x0007
	REG_RFA3SS       uint16 = 0x0008
	REG_ENDA3        uint16 = 0x0009
	REG_RFA4SS       uint16 = 0x000A
	REG_I2CSS        uint16 = 0x000B
	REG_LOCK_CCFILE  uint16 = 0x000C
	REG_MB_MODE      uint16 = 0x000D
	REG_MB_WDG       uint16 = 0x000E
	REG_LOCK_CFG     uint16 = 0x000F
	REG_LOCK_DSFID   uint16 = 0x0010
	REG_LOCK_AFI     uint16 = 0x0011
	REG_DSFID        uint16 = 0x0012
	REG_AFI          uint16 = 0x0013
	REG_MEM_SIZE_LSB uint16 = 0x0014
	REG_MEM_SIZE_MSB uint16 = 0x0015
	REG_BLK_SIZE     uint16 = 0x0016
	REG_IC_REF       uint16 = 0x0017
	REG_UID          uint16 = 0x0018
	REG_IC_REV       uint16 = 0x0020
	REG_I2C_PWD      uint16 = 0x0900
)

// Dynamic registers and fast transfer mode mailbox, accessible via
// AddressUser
const (
	REG_GPO_CTRL_DYN uint16 = 0x2000
	REG_EH_CTRL_DYN  uint16 = 0x2002
	REG_RF_MNGT_DYN  uint16 = 0x2003
	REG_I2C_SSO_DYN  uint16 = 0x2004
	REG_IT_STS_DYN   uint16 = 0x2005
	REG_MB_CTRL_DYN  uint16 = 0x2006
	REG_MB_LEN_DYN   uint16 = 0x2007
	MAILBOX_START    uint16 = 0x2008
)

// The first byte of the ISO 15693 UID is always 0xE0, the second is the IC
// manufacturer code of STMicroelectronics
const (
	uidISO15693Prefix   uint8 = 0xE0
	uidSTManufacturerID uint8 = 0x02
)

// The chip accepts I2C writes of up to 256 bytes, this driver uses a smaller
// chunk to keep its fixed transfer buffer small
const writeChunkSize = 32
