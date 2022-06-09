package main

import (
	"encoding/json"
	"fmt"
	"github.com/r3labs/sse/v2"
	"log"
	"net"
	"time"
)

type Event struct {
	Id         string     `json:"id"`
	Capability string     `json:"capability"`
	Value      EventValue `json:"value"`
}

type EventValue struct {
	EventType string `json:"type"`
	Value     any    `json:"value"`
}

type LedData struct {
	Br byte
	R  byte
	G  byte
	B  byte
}

func main() {
	client := sse.NewClient("https://iot.mtdl.ru/events")

	ledCh := make(chan LedData)
	go ledBus(ledCh)

	acCh := make(chan []byte)
	go acBus(acCh)

	ledStrip := NewLed()
	ac := newAC()

	client.Subscribe("messages", func(msg *sse.Event) {
		// Got some data!
		var event Event
		_ = json.Unmarshal(msg.Data, &event)

		log.Printf("%v", event)

		if event.Id == "led_strip" {
			handleLed(event, ledStrip)
			go ledStrip.render(ledCh)
		} else if event.Id == "ac" {
			handleIR(event, ac)
			acCh <- ac.render()
		}
	})

}

func connectLed(ch chan *net.UDPConn) {
	for {
		conn, err := net.ResolveUDPAddr("udp4", "led.local:9000")
		if err != nil {
			log.Print("Led unavailable, reconnecting after 2s...")
			<-time.After(2 * time.Second)
			continue
		}
		bus, _ := net.DialUDP("udp4", nil, conn)
		defer bus.Close()
		ch <- bus
	}
}

func ledBus(ch chan LedData) {
	connCh := make(chan *net.UDPConn)
	go connectLed(connCh)
	conn := <-connCh
	fmt.Printf("The LED connected is %s\n", conn.RemoteAddr().String())
	<-ch
	for {
		data := <-ch
		if data.Br == 0 {
			for i := 0; i < 5; i++ {
				log.Printf("Led crutch send data: %v", data)
				sendData := make([]byte, 901)
				sendData[0] = data.Br

				for i := 0; i < 300; i++ {
					sendData[(i*3)+1] = data.R
					sendData[(i*3)+2] = data.G
					sendData[(i*3)+3] = data.B
				}

				_, _ = conn.Write(sendData)
			}
		}
		sendData := make([]byte, 901)
		sendData[0] = data.Br

		for i := 0; i < 300; i++ {
			sendData[(i*3)+1] = data.R
			sendData[(i*3)+2] = data.G
			sendData[(i*3)+3] = data.B
		}

		_, _ = conn.Write(sendData)
	}
}

func connectAC(ch chan *net.UDPConn) {
	for {
		conn, err := net.ResolveUDPAddr("udp4", "ir.local:9000")
		if err != nil {
			log.Print("AC unavailable, reconnecting after 2s...")
			<-time.After(2 * time.Second)
			continue
		}
		bus, _ := net.DialUDP("udp4", nil, conn)
		defer bus.Close()
		ch <- bus
	}
}

func acBus(ch chan []byte) {
	connCh := make(chan *net.UDPConn)
	go connectAC(connCh)
	conn := <-connCh
	fmt.Printf("The AC connected is %s\n", conn.RemoteAddr().String())
	<-ch
	for {
		data := <-ch
		log.Printf("Sending AC data: %v", data)
		_, _ = conn.Write(data)
	}
}

func handleLed(event Event, ledStrip *Led) {
	switch event.Capability {
	case "devices.capabilities.on_off":
		switch v := event.Value.Value.(type) {
		case bool:
			ledStrip.setState(v)
			break
		default:
			log.Printf("Value invalid (%v_%v_%v_%v)", event.Id, event.Capability, event.Value.EventType, event.Value.Value)
		}
		break
	case "devices.capabilities.color_setting":
		switch v := event.Value.Value.(type) {
		case string:
			ledStrip.setScene(v)
			break
		case float64:
			ledStrip.setColor(v)
		}
	case "devices.capabilities.range":
		if event.Value.EventType == "brightness" {
			switch v := event.Value.Value.(type) {
			case float64:
				ledStrip.setBrightness(v)
				break
			}
		}
		break
	}
}

func handleIR(event Event, ac *AC) {
	switch event.Capability {
	case "devices.capabilities.on_off":
		switch v := event.Value.Value.(type) {
		case bool:
			ac.setState(v)
			break
		default:
			log.Printf("Value invalid (%v_%v_%v_%v)", event.Id, event.Capability, event.Value.EventType, event.Value.Value)
		}
		break
	case "devices.capabilities.range":
		if event.Value.EventType == "temperature" {
			switch v := event.Value.Value.(type) {
			case float64:
				ac.setTemp(v)
				break
			}
		}
		break
	case "devices.capabilities.mode":
		if event.Value.EventType == "thermostat" {
			switch v := event.Value.Value.(type) {
			case string:
				ac.setMode(v)
				break
			}
		}

		if event.Value.EventType == "fan_speed" {
			switch v := event.Value.Value.(type) {
			case string:
				ac.setFanSpeed(v)
				break
			}
		}
	}
}

func calcColor(color int) (red, green, blue uint8) {
	blue = uint8(color & 0xFF)
	green = uint8((color >> 8) & 0xFF)
	red = uint8((color >> 16) & 0xFF)

	return red, green, blue
}
