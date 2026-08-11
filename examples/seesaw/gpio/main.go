package main

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/seesaw"
)

// example using an Adafruit ATtiny1616 Breakout with seesaw (4747/5690) as a general purpose
// I2C GPIO/analog expander: https://learn.adafruit.com/adafruit-attiny817-seesaw
const (
	ledPin    = 15 // digital output, e.g. driving an LED through a resistor
	buttonPin = 14 // digital input with a pull-up, e.g. a button to ground
	sensorPin = 5  // ADC input, e.g. a potentiometer wiper
	pwmPin    = 11 // PWM output, e.g. dimming a second LED
)

func main() {
	// This assumes you are using an Adafruit QT Py RP2040 for its Stemma QT connector
	// https://www.adafruit.com/product/4900
	i2c := machine.I2C1
	i2c.Configure(machine.I2CConfig{
		SCL: machine.I2C1_QT_SCL_PIN,
		SDA: machine.I2C1_QT_SDA_PIN,
	})

	// seesaw.New defaults Address to 0x49, which matches this board.
	dev := seesaw.New(i2c)

	if err := dev.SetPinMode(ledPin, seesaw.Output); err != nil {
		panic(err)
	}
	if err := dev.SetPinMode(buttonPin, seesaw.InputPullup); err != nil {
		panic(err)
	}

	on := false
	var brightness uint16
	for {
		on = !on
		if err := dev.WritePin(ledPin, on); err != nil {
			panic(err)
		}

		pressed, err := dev.ReadPin(buttonPin)
		if err != nil {
			panic(err)
		}
		if !pressed { // pull-up: a closed button pulls the pin low
			println("button pressed")
		}

		level, err := dev.ReadADC(sensorPin)
		if err != nil {
			panic(err)
		}
		println("sensor:", level)

		brightness += 4096
		if err := dev.SetPWM(pwmPin, brightness); err != nil {
			panic(err)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
