package cwc

import rpio "github.com/stianeikeland/go-rpio"


type PiGPIO struct {
	config ConfigMap
	output rpio.Pin
	input rpio.Pin
}

func (g *PiGPIO) Open() error {
	err := rpio.Open()
	if err != nil {
		return err
	}
	g.output = rpio.Pin(17) // header pin 11 BCM17
	g.output.Output()
	g.output.Low()

    g.input = rpio.Pin(27) // header pin 13 BCM27
    g.input.Input()
    g.input.PullUp()

    return nil
}

func (g *PiGPIO) SetConfig(key string, value string) {
	g.config[key] = value
}

func (g *PiGPIO) ConfigMap() ConfigMap {
	return g.config
}

func (g *PiGPIO) Bit() uint8 {
	if g.input.Read() ==  rpio.High {
		return 0x01
	} else {
		return 0x00
	}
}

func (g *PiGPIO) SetBit(bit0 uint8) {
	if bit0 & 0x01 > 0 {
		g.output.High()
	} else {
		g.output.Low()
	}
}


func (g *PiGPIO) Close() {
	// pass
}

