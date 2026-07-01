package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers/is31fl3741"
)

func main() {
	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err.Error())
		return
	}

	// Create driver for Adafruit 13x9 PWM RGB LED Matrix Driver (IS31FL3741
	// QT): https://www.adafruit.com/product/5201
	display := is31fl3741.NewAdafruitRGBMatrixQT13x9(bus, is31fl3741.I2C_ADDRESS_GND)

	err = display.Configure()
	if err != nil {
		println("could not configure is31fl3741 driver:", err.Error())
		return
	}

	// Tone the brightness down, full global current is very bright
	err = display.SetGlobalCurrent(0x30)
	if err != nil {
		println("could not set global current:", err.Error())
		return
	}

	colors := []color.RGBA{
		{R: 0xFF},
		{G: 0xFF},
		{B: 0xFF},
	}

	// Wipe the matrix column by column with red, green and blue in turn
	for i := 0; ; i++ {
		c := colors[i%len(colors)]
		for x := int16(0); x < 13; x++ {
			for y := int16(0); y < 9; y++ {
				display.SetPixel(x, y, c)
			}
			err = display.Display()
			if err != nil {
				println("could not update display:", err.Error())
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
