package main

import (
	"log"
)

type AC struct {
	State      bool
	Temp       float64
	FanSpeed   uint8
	Thermostat uint8
}

func newAC() *AC {
	return &AC{
		State:      false,
		Temp:       21,
		FanSpeed:   3,
		Thermostat: 0,
	}
}

func (l *AC) setState(value bool) {
	log.Printf("Set ac state: %v", value)
	l.State = value
}

func (l *AC) setTemp(value float64) {
	log.Printf("Set ac temp: %v", value)
	l.Temp = value
}

func (l *AC) setFanSpeed(value string) {
	switch value {
	case "low":
		l.FanSpeed = 0
		break
	case "medium":
		l.FanSpeed = 1
		break
	case "high":
		l.FanSpeed = 2
		break
	case "auto":
		l.FanSpeed = 3
	}
	log.Printf("Set ac fan speed: %v", l.FanSpeed)
}

func (l *AC) setMode(value string) {
	switch value {
	case "cool":
		l.Thermostat = 1
		break
	case "dry":
		l.Thermostat = 2
		break
	case "heat":
		l.Thermostat = 3
		break
	case "fan_only":
		l.Thermostat = 4
		break
	case "auto":
		l.Thermostat = 0
	}
	log.Printf("Set ac mode: %v", l.Thermostat)
}

func (l *AC) render() []byte {
	data := make([]byte, 4)
	if l.State {
		data[0] = 1
	} else {
		data[0] = 0
	}

	data[1] = byte(l.Temp)
	data[2] = l.FanSpeed
	data[3] = l.Thermostat

	return data
}
