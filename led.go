package main

import (
	"github.com/lucasb-eyer/go-colorful"
	"log"
	"time"
)

type Led struct {
	State      bool
	Color      colorful.Color
	Scene      string
	Brightness float64

	fromColor    colorful.Color
	currentColor colorful.Color

	fromBrightness    float64
	currentBrightness float64
	toBrightness      float64

	tColor      float64
	tBrightness float64
}

func NewLed() *Led {
	color := colorful.Color{R: 255, G: 10, B: 10}
	return &Led{
		State:             false,
		Color:             color,
		Scene:             "none",
		Brightness:        0,
		fromColor:         color,
		currentColor:      color,
		fromBrightness:    0,
		currentBrightness: 0,
		toBrightness:      0,

		tColor:      1,
		tBrightness: 1,
	}
}

func (l *Led) setState(value bool) {
	log.Printf("Set led state: %v", value)
	l.State = value
	if l.State {
		l.toBrightness = l.Brightness
	} else {
		l.toBrightness = 0
	}

	l.tBrightness = 0
	l.fromBrightness = l.currentBrightness
}

func (l *Led) setBrightness(value float64) {
	log.Printf("Set led brightness: %v", value)
	l.Brightness = (value / 100) * 255
	if l.State {
		l.toBrightness = l.Brightness
	}

	l.tBrightness = 0
	l.fromBrightness = l.currentBrightness
}

func (l *Led) setScene(value string) {
	l.Scene = value
}

func (l *Led) setColor(value float64) {
	log.Printf("Set led color: %v", value)
	r, g, b := parseColor(int(value))
	l.Color = colorful.Color{R: r, G: g, B: b}
	l.tColor = 0
	l.fromColor = l.currentColor
}

func (l *Led) render(ch chan LedData) {
	renderSolid(l, ch)
}

func renderSolid(l *Led, ch chan LedData) {
	for l.tColor < 1 || l.tBrightness < 1 {
		currentColor := l.fromColor.BlendRgb(l.Color, l.tColor)
		currentBrightness := blendFloat(l.fromBrightness, l.toBrightness, l.tBrightness)

		l.currentBrightness = currentBrightness
		l.currentColor = currentColor

		if l.tBrightness < 1 || l.toBrightness != l.Brightness {
			l.tBrightness += 0.007
		}

		if l.tColor < 1 {
			l.tColor += 0.007
		}

		r, g, b := currentColor.RGB255()
		ch <- LedData{
			Br: byte(currentBrightness),
			R:  r,
			G:  g,
			B:  b,
		}
		<-time.After(2 * time.Millisecond)
	}

	l.fromColor = l.Color
	l.fromBrightness = l.Brightness

	r, g, b := l.Color.RGB255()
	ch <- LedData{
		Br: byte(l.toBrightness),
		R:  r,
		G:  g,
		B:  b,
	}
}

func parseColor(color int) (red, green, blue float64) {
	blue = float64(color & 0xFF)
	green = float64((color >> 8) & 0xFF)
	red = float64((color >> 16) & 0xFF)

	return red / 255, green / 255, blue / 255
}

func clamp(x float64, barr float64) uint8 {
	if x > barr {
		return uint8(barr)
	}
	if x > 0 {
		return uint8(x)
	}
	return 0
}

func blendFloat(from float64, to float64, t float64) float64 {
	return from + t*(to-from)
}
