package mmc5983

// Registers definitions
const (
	X_OUT_0_REG    uint8 = 0x00
	X_OUT_1_REG    uint8 = 0x01
	Y_OUT_0_REG    uint8 = 0x02
	Y_OUT_1_REG    uint8 = 0x03
	Z_OUT_0_REG    uint8 = 0x04
	Z_OUT_1_REG    uint8 = 0x05
	XYZ_OUT_2_REG  uint8 = 0x06
	T_OUT_REG      uint8 = 0x07
	STATUS_REG     uint8 = 0x08
	INT_CTRL_0_REG uint8 = 0x09
	INT_CTRL_1_REG uint8 = 0x0a
	INT_CTRL_2_REG uint8 = 0x0b
	INT_CTRL_3_REG uint8 = 0x0c
	PROD_ID_REG    uint8 = 0x2f
)

// Constants definitions
const (
	I2C_ADDR uint16 = 0x30
	PROD_ID  uint8  = 0x30
)

// Bits definitions
const (
	BITS_MEAS_M_DONE      = (1 << 0)
	BITS_MEAS_T_DONE      = (1 << 1)
	BITS_OTP_READ_DONE    = (1 << 4)
	BITS_TM_M             = (1 << 0)
	BITS_TM_T             = (1 << 1)
	BITS_INT_MEAS_DONE_EN = (1 << 2)
	BITS_SET_OPERATION    = (1 << 3)
	BITS_RESET_OPERATION  = (1 << 4)
	BITS_AUTO_SR_EN       = (1 << 5)
	BITS_OTP_READ         = (1 << 6)
	BITS_BW0              = (1 << 0)
	BITS_BW1              = (1 << 1)
	BITS_X_INHIBIT        = (1 << 2)
	BITS_YZ_INHIBIT       = (3 << 3)
	BITS_SW_RST           = (1 << 7)
	BITS_CM_FREQ_0        = (1 << 0)
	BITS_CM_FREQ_1        = (1 << 1)
	BITS_CM_FREQ_2        = (1 << 2)
	BITS_CMM_EN           = (1 << 3)
	BITS_PRD_SET_0        = (1 << 4)
	BITS_PRD_SET_1        = (1 << 5)
	BITS_PRD_SET_2        = (1 << 6)
	BITS_EN_PRD_SET       = (1 << 7)
	BITS_ST_ENP           = (1 << 1)
	BITS_ST_ENM           = (1 << 2)
	BITS_SPI_3W           = (1 << 6)
	BITS_X2_MASK          = (3 << 6)
	BITS_Y2_MASK          = (3 << 4)
	BITS_Z2_MASK          = (3 << 2)
	BITS_XYZ_0_SHIFT      = 10
	BITS_XYZ_1_SHIFT      = 2
)
