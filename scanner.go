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
	ps    struct {
		mux  *sync.Mutex
		subs map[chan string]struct{}
	}
}

// NewScanner creates a new XAir device scanner.
func NewScanner() Scanner {
	xairs := make(map[string]XAir)

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(err.Error())
	}

	mux := sync.Mutex{}

	ps := struct {
		mux  *sync.Mutex
		subs map[chan string]struct{}
	}{
		&sync.Mutex{},
		make(map[chan string]struct{}),
	}

	return Scanner{xairs, conn, &mux, ps}
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

// Subscribe to updates when XAir devices are detected.
func (scanner Scanner) Subscribe() chan string {
	sub := make(chan string)

	scanner.ps.mux.Lock()
	scanner.ps.subs[sub] = struct{}{}
	scanner.ps.mux.Unlock()

	Log.Info.Printf("Subscribing %v to XAir scanner.", sub)
	return sub
}

// Unsubscribe from updates when XAir devices are detected.
func (scanner Scanner) Unsubscribe(sub chan string) {
	scanner.ps.mux.Lock()
	delete(scanner.ps.subs, sub)
	scanner.ps.mux.Unlock()

	Log.Info.Printf("Unsubscribing %v from XAir scanner.", sub)
}

func (scanner Scanner) publish(pub chan string) {
	for name := range pub {
		for sub := range scanner.ps.subs {
			sub <- name
		}
	}
}

// List detected XAir devices.
func (scanner Scanner) List() []string {
	scanner.mux.Lock()
	defer scanner.mux.Unlock()
	xairs := make([]string, 0, len(scanner.xairs))
	for name := range scanner.xairs {
		xairs = append(xairs, name)
	}
	return xairs
}

func (scanner Scanner) register(address string, name string, publish chan string, terminate chan string) {
	if _, ok := scanner.xairs[name]; !ok {
		Log.Info.Printf("Register %s at %s.", name, address)
		xair := NewXAir(address, name, []int{2, 3, 5})
		go xair.Start(terminate)
		scanner.mux.Lock()
		scanner.xairs[name] = xair
		scanner.mux.Unlock()
		publish <- name
	}
}

func (scanner Scanner) unregister(publish chan string, terminate chan string) {
	for name := range terminate {
		if xair, ok := scanner.xairs[name]; ok {
			Log.Info.Printf("Unregister %s.", name)
			xair.Close()
			scanner.mux.Lock()
			delete(scanner.xairs, name)
			scanner.mux.Unlock()
			publish <- name
		}
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

	publish := make(chan string)
	go scanner.publish(publish)
	defer close(publish)

	terminate := make(chan string)
	go scanner.unregister(publish, terminate)
	defer close(terminate)

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
		if address == "0.0.0.0" {
			continue
		}
		if err != nil {
			panic(err.Error())
		}

		name, err := msg.Arguments[1].ReadString()
		if err != nil {
			panic(err.Error())
		}

		scanner.register(fmt.Sprintf("%s:10024", address), name, publish, terminate)
	}
}
