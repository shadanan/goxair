package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Scanner scans for XAir devices on the network.
type Scanner struct {
	xairs map[string]XAir
	conn  *net.UDPConn
	mux   *sync.Mutex
}

// NewScanner creates a new XAir device scanner.
func NewScanner() Scanner {
	xairs := make(map[string]XAir)

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(err.Error())
	}

	mux := sync.Mutex{}

	return Scanner{xairs, conn, &mux}
}

// Start scanning for XAir devices.
func (scanner Scanner) Start() {
	stopBroadcast := make(chan struct{})
	defer close(stopBroadcast)
	go scanner.broadcast(stopBroadcast)

	scanner.detect()
}

// Stop scanning for XAir devices.
func (scanner Scanner) Stop() {
	Log.Info.Printf("Stopping scanner.")
	scanner.conn.Close()
}

func (scanner Scanner) register(address string, name string) {
	scanner.mux.Lock()
	defer scanner.mux.Unlock()

	if _, ok := scanner.xairs[name]; !ok {
		Log.Info.Printf("Register %s at %s.", name, address)
		xair := NewXAir(address, name, []int{2, 3, 5})
		go xair.Start()
		scanner.xairs[name] = xair
	}
}

func (scanner Scanner) unregister(name string) {
	scanner.mux.Lock()
	defer scanner.mux.Unlock()

	if xair, ok := scanner.xairs[name]; ok {
		Log.Info.Printf("Unregister %s.", name)
		xair.Close()
		delete(scanner.xairs, name)
	}
}

func (scanner Scanner) broadcast(stop chan struct{}) {
	defer Log.Info.Printf("Scanner broadcast terminated.")

	msg := Message{Address: "/xinfo"}
	baddr := &net.UDPAddr{IP: net.IPv4bcast, Port: 10024}

	for {
		_, err := scanner.conn.WriteTo(msg.Bytes(), baddr)
		if err != nil {
			panic(err.Error())
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (scanner Scanner) detect() {
	defer Log.Info.Printf("Scanner detect terminated.")

	for {
		data := make([]byte, 65535)

		_, err := scanner.conn.Read(data)
		if err != nil {
			return
		}

		msg, err := ParseMessage(data)
		if err != nil {
			panic(err.Error())
		}

		address, err := msg.Arguments[0].ReadString()
		if err != nil {
			panic(err.Error())
		}

		name, err := msg.Arguments[1].ReadString()
		if err != nil {
			panic(err.Error())
		}

		scanner.register(fmt.Sprintf("%s:10024", address), name)
	}
}
