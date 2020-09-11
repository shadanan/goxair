package scanner

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shadanan/goxair/log"
	"github.com/shadanan/goxair/osc"
	"github.com/shadanan/goxair/xair"
)

// Scanner scans for XAir devices on the network.
type Scanner struct {
	xairs map[string]xair.XAir
	conn  *net.UDPConn
	mux   *sync.Mutex
	ps    pubsub
}

// NewScanner creates a new XAir device scanner.
func NewScanner() Scanner {
	xairs := make(map[string]xair.XAir)

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(err.Error())
	}

	mux := &sync.Mutex{}
	ps := newPubsub()

	return Scanner{xairs, conn, mux, ps}
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
	log.Info.Printf("Stopping scanner.")
	scanner.conn.Close()
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

// Get the XAir device by name.
func (scanner Scanner) Get(name string) xair.XAir {
	return scanner.xairs[name]
}

// Subscribe to updates when XAir devices are detected.
func (scanner Scanner) Subscribe() chan string {
	sub := make(chan string, 10)
	scanner.ps.sub <- sub
	log.Info.Printf("Subscribing %v to XAir scanner.", sub)
	return sub
}

// Unsubscribe from updates when XAir devices are detected.
func (scanner Scanner) Unsubscribe(sub chan string) {
	scanner.ps.unsub <- sub
	log.Info.Printf("Unsubscribing %v from XAir scanner.", sub)
}

func (scanner Scanner) register(address string, name string, terminate chan string) {
	if _, ok := scanner.xairs[name]; !ok {
		log.Info.Printf("Register %s at %s.", name, address)
		xa := xair.NewXAir(address, name, []int{2, 3, 5})
		go xa.Start(terminate)
		scanner.mux.Lock()
		scanner.xairs[name] = xa
		scanner.mux.Unlock()
		scanner.ps.publish(name)
	}
}

func (scanner Scanner) unregister(terminate chan string) {
	for name := range terminate {
		if xa, ok := scanner.xairs[name]; ok {
			log.Info.Printf("Unregister %s.", name)
			xa.Close()
			scanner.mux.Lock()
			delete(scanner.xairs, name)
			scanner.mux.Unlock()
			scanner.ps.publish(name)
		}
	}
}

func (scanner Scanner) broadcast(stop chan struct{}) {
	defer log.Info.Printf("Scanner broadcast terminated.")

	msg := osc.Message{Address: "/xinfo"}
	baddr := &net.UDPAddr{IP: net.IPv4bcast, Port: 10024}

	for {
		_, err := scanner.conn.WriteTo(msg.Bytes(), baddr)
		if err != nil {
			log.Error.Printf("Failed to broadcast: %s", err)
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

type pubsub struct {
	pub   chan string
	sub   chan chan string
	unsub chan chan string
	st    chan struct{}
}

func newPubsub() pubsub {
	return pubsub{
		pub:   make(chan string),
		sub:   make(chan chan string),
		unsub: make(chan chan string),
		st:    make(chan struct{}),
	}
}

func (ps pubsub) start() {
	subs := make(map[chan string]bool)

	for {
		select {
		case name := <-ps.pub:
			for sub := range subs {
				sub <- name
			}
		case sub := <-ps.sub:
			subs[sub] = true
		case sub := <-ps.unsub:
			delete(subs, sub)
			close(sub)
		case <-ps.st:
			return
		}
	}
}

func (ps pubsub) publish(name string) {
	ps.pub <- name
}

func (ps pubsub) stop() {
	close(ps.st)
}

func (scanner Scanner) detect() {
	defer log.Info.Printf("Scanner detect terminated.")

	go scanner.ps.start()
	defer scanner.ps.stop()

	terminate := make(chan string, 10)
	go scanner.unregister(terminate)
	defer close(terminate)

	for {
		data := make([]byte, 65535)

		_, err := scanner.conn.Read(data)
		if err != nil {
			return
		}

		msg, err := osc.ParseMessage(data)
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

		scanner.register(fmt.Sprintf("%s:10024", address), name, terminate)
	}
}
