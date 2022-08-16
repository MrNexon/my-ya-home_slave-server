package main

import (
	"encoding/json"
	"log"
	"net"
	"smartHome/stream"
	"time"
)

type Event struct {
	Id         string     `json:"id"`
	Sync       bool       `json:"sync"`
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
	ledCh := make(chan LedData)
	go ledBus(ledCh)

	acCh := make(chan []byte)
	go acBus(acCh)

	ledStrip := NewLed()
	ac := newAC()

	events := make(chan Event)
	go connectSSE(events)

	for {
		event := <-events

		//log.Printf("%v", event)

		if event.Id == "led_strip" {
			handleLed(event, ledStrip)
			if !event.Sync {
				go ledStrip.render(ledCh)
			}
		} else if event.Id == "ac" {
			handleIR(event, ac)
			if !event.Sync {
				acCh <- ac.render()
			}
		}
	}

}

func connectSSE(events chan Event) {
	log.Println("Connecting to event server")
	s := stream.NewStream("https://iot.mtdl.ru/events")
	s.Connect()
	for {
		select {
		case data := <-s.Data:
			dataStr := string(data)
			if dataStr != ":ping" {
				var event Event
				_ = json.Unmarshal(data, &event)
				events <- event
			}
		case err := <-s.Error:
			log.Printf("Error: %v", err)
		case <-s.Exit:
			log.Println("Stream closed.")
		}
	}
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
	log.Printf("The LED connected is %s\n", conn.RemoteAddr().String())
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
	log.Printf("The AC connected is %s\n", conn.RemoteAddr().String())
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
			ledStrip.setState(v, event.Sync)
			break
		default:
			log.Printf("Value invalid (%v_%v_%v_%v)", event.Id, event.Capability, event.Value.EventType, event.Value.Value)
		}
		break
	case "devices.capabilities.color_setting":
		switch v := event.Value.Value.(type) {
		case string:
			ledStrip.setScene(v, event.Sync)
			break
		case float64:
			ledStrip.setColor(v, event.Sync)
		}
	case "devices.capabilities.range":
		if event.Value.EventType == "brightness" {
			switch v := event.Value.Value.(type) {
			case float64:
				ledStrip.setBrightness(v, event.Sync)
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
			ac.setState(v, event.Sync)
			break
		default:
			log.Printf("Value invalid (%v_%v_%v_%v)", event.Id, event.Capability, event.Value.EventType, event.Value.Value)
		}
		break
	case "devices.capabilities.range":
		if event.Value.EventType == "temperature" {
			switch v := event.Value.Value.(type) {
			case float64:
				ac.setTemp(v, event.Sync)
				break
			}
		}
		break
	case "devices.capabilities.mode":
		if event.Value.EventType == "thermostat" {
			switch v := event.Value.Value.(type) {
			case string:
				ac.setMode(v, event.Sync)
				break
			}
		}

		if event.Value.EventType == "fan_speed" {
			switch v := event.Value.Value.(type) {
			case string:
				ac.setFanSpeed(v, event.Sync)
				break
			}
		}
	}
}
