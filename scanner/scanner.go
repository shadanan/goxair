package scanner

import (
	"fmt"
	"net"
	"time"

	"github.com/shadanan/goxair/log"
	"github.com/shadanan/goxair/osc"
	"github.com/shadanan/goxair/pubsub"
	"github.com/shadanan/goxair/xair"
)

// Scanner scans for XAir devices on the network.
type Scanner struct {
	ps    pubsub.String
	reg   chan regmsg
	unreg chan string
	list  chan chan string
	get   chan getmsg
	st    chan struct{}
}

type regmsg struct {
	address string
	name    string
}

type getmsg struct {
	ch   chan xair.XAir
	name string
}

// NewScanner creates a new XAir device scanner.
func NewScanner(buffer int) Scanner {
	return Scanner{
		ps:    pubsub.NewString(),
		reg:   make(chan regmsg, buffer),
		unreg: make(chan string, buffer),
		list:  make(chan chan string, buffer),
		get:   make(chan getmsg, buffer),
		st:    make(chan struct{}),
	}
}

// Start scanning for XAir devices.
func (scanner Scanner) Start() {
	defer log.Info.Printf("XAir scanner stopped.")

	go scanner.ps.Start()
	defer scanner.ps.Stop()

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(err.Error())
	}

	stopBroadcast := make(chan struct{})
	defer close(stopBroadcast)
	go broadcast(conn, stopBroadcast)
	go detect(conn, scanner.reg)

	xairs := make(map[string]xair.XAir)

	for {
		select {
		case reg := <-scanner.reg:
			if _, ok := xairs[reg.name]; !ok {
				log.Info.Printf("Register %s at %s.", reg.name, reg.address)
				xa := xair.NewXAir(reg.address, reg.name, []int{2, 3, 5})
				go xa.Start(scanner.unreg)
				xairs[reg.name] = xa
				scanner.ps.Publish(reg.name)
			}
		case name := <-scanner.unreg:
			if xa, ok := xairs[name]; ok {
				log.Info.Printf("Unregister %s.", name)
				xa.Close()
				delete(xairs, name)
				scanner.ps.Publish(name)
			}
		case ch := <-scanner.list:
			for name := range xairs {
				ch <- name
			}
			close(ch)
		case req := <-scanner.get:
			req.ch <- xairs[req.name]
			close(req.ch)
		case <-scanner.st:
			conn.Close()
			return
		}
	}
}

// Stop scanning for XAir devices.
func (scanner Scanner) Stop() {
	close(scanner.st)
}

// List detected XAir devices.
func (scanner Scanner) List() []string {
	ch := make(chan string)
	scanner.list <- ch

	xairs := make([]string, 0)
	for xa := range ch {
		xairs = append(xairs, xa)
	}

	return xairs
}

// Get the XAir device by name.
func (scanner Scanner) Get(name string) xair.XAir {
	ch := make(chan xair.XAir)
	scanner.get <- getmsg{ch, name}
	return <-ch
}

// Subscribe to updates when XAir devices are detected.
func (scanner Scanner) Subscribe() chan string {
	sub := scanner.ps.Subscribe()
	log.Info.Printf("Subscribed %v to XAir scanner.", sub)
	return sub
}

// Unsubscribe from updates when XAir devices are detected.
func (scanner Scanner) Unsubscribe(sub chan string) {
	scanner.ps.Unsubscribe(sub)
	log.Info.Printf("Unsubscribed %v from XAir scanner.", sub)
}

func broadcast(conn *net.UDPConn, stop chan struct{}) {
	defer log.Info.Printf("Scanner broadcast terminated.")

	msg := osc.Message{Address: "/xinfo"}
	baddr := &net.UDPAddr{IP: net.IPv4bcast, Port: 10024}

	for {
		_, err := conn.WriteTo(msg.Bytes(), baddr)
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

func detect(conn *net.UDPConn, reg chan regmsg) {
	defer log.Info.Printf("Scanner detect terminated.")

	for {
		data := make([]byte, 65535)

		_, err := conn.Read(data)
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

		reg <- regmsg{
			address: fmt.Sprintf("%s:10024", address),
			name:    name,
		}
	}
}
