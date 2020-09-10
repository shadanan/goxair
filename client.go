package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// XAir client
type XAir struct {
	conn   *net.UDPConn
	ps     pubsub
	cache  map[string]Message
	name   string
	meters Arguments
}

type pubsub struct {
	pub   chan Message
	sub   chan chan Message
	unsub chan chan Message
	st    chan struct{}
}

// NewXAir creates a new XAir client.
func NewXAir(address string, name string, meters []int) XAir {
	raddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		panic(err.Error())
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		panic(err.Error())
	}

	ps := newPubsub()
	cache := make(map[string]Message)

	meterArgs := make([]Argument, len(meters))
	for i, meter := range meters {
		meterArgs[i] = String(fmt.Sprintf("/meters/%d", meter))
	}

	return XAir{conn, ps, cache, name, meterArgs}
}

// Start polling for messages and publish when they are received.
func (xair XAir) Start(terminate chan string) {
	defer Log.Info.Printf("Connection to %s closed.", xair.name)

	go xair.ps.start()
	defer xair.ps.stop()

	updateSub := xair.Subscribe()
	go xair.update(updateSub, terminate)
	defer xair.Unsubscribe(updateSub)

	stopRefresh := make(chan struct{})
	go xair.refresh(stopRefresh)
	defer close(stopRefresh)

	for {
		msg, err := xair.receive()
		if errors.Is(err, ErrConnClosed) {
			return
		}
		xair.ps.publish(msg)
	}
}

func (xair XAir) refresh(stop chan struct{}) {
	defer Log.Debug.Printf("%s refresh goroutine terminated.", xair.name)

	for {
		xair.send(Message{Address: "/xremote"})
		for _, meter := range xair.meters {
			xair.send(Message{Address: "/meters", Arguments: Arguments{meter}})
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (xair XAir) update(sub chan Message, terminate chan string) {
	defer Log.Debug.Printf("%s update goroutine terminated.", xair.name)

	timeout := time.NewTicker(1 * time.Second)
	for {
		select {
		case v := <-timeout.C:
			Log.Info.Printf("Timeout from %s: %s.", xair.name, v)
			terminate <- xair.name
		case msg, ok := <-sub:
			if !ok {
				return
			}

			timeout.Reset(1 * time.Second)
			if !strings.HasPrefix(msg.Address, "/meters/") {
				Log.Info.Printf("Received from %s: %s", xair.name, msg)
				xair.cache[msg.Address] = msg
			}
		}
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xair XAir) Close() {
	Log.Info.Printf("Closing connection to %s.", xair.name)
	xair.conn.Close()
}

// Subscribe to messages received from the XAir device.
func (xair XAir) Subscribe() chan Message {
	ch := xair.ps.subscribe()
	Log.Debug.Printf("Subscribing %p to %s.", ch, xair.name)
	return ch
}

// Unsubscribe from messages from the XAir device.
func (xair XAir) Unsubscribe(ch chan Message) {
	Log.Debug.Printf("Unsubscribing %p from %s.", ch, xair.name)
	xair.ps.unsubscribe(ch)
}

// Get the value of an address on the XAir device.
func (xair XAir) Get(address string) (Message, error) {
	if msg, ok := xair.cache[address]; ok {
		Log.Info.Printf("Get on %s (cached): %s", xair.name, msg)
		return msg, nil
	}
	sub := xair.Subscribe()
	defer xair.Unsubscribe(sub)
	xair.send(Message{Address: address})
	for {
		select {
		case msg := <-sub:
			if msg.Address == address {
				Log.Info.Printf("Get on %s: %s", xair.name, msg)
				return msg, nil
			}
		case <-time.After(1 * time.Second):
			Log.Info.Printf("Get timed out on %s: %s", xair.name, address)
			return Message{}, ErrTimeout
		}
	}
}

// Set the value of an address on the XAir device.
func (xair XAir) Set(address string, arguments Arguments) {
	msg := Message{Address: address, Arguments: arguments}
	Log.Info.Printf("Set on %s: %s", xair.name, msg)
	xair.cache[msg.Address] = msg
	xair.send(msg)
}

func (xair XAir) send(msg Message) {
	Log.Debug.Printf("Send to %s: %s", xair.name, msg)
	_, err := xair.conn.Write(msg.Bytes())
	if err != nil {
		Log.Error.Printf("Cannot send to %s because connection is closed.", xair.name)
	}
}

func (xair XAir) receive() (Message, error) {
	data := make([]byte, 65535)
	_, err := xair.conn.Read(data)
	if err != nil {
		return Message{}, ErrConnClosed
	}

	msg, err := ParseMessage(data)
	if err != nil {
		panic(err.Error())
	}

	if strings.HasPrefix(msg.Address, "/meters/") {
		blob, err := msg.Arguments[0].ReadBlob()
		if err != nil {
			panic(err.Error())
		}
		msg.Arguments = decodeMeter(blob)
	}

	return msg, nil
}

func decodeMeter(blob []byte) []Argument {
	buffer := bytes.NewBuffer(blob)

	var size int32
	err := binary.Read(buffer, binary.LittleEndian, &size)
	if err != nil {
		panic(err.Error())
	}

	meter := make([]Argument, size)
	for i := 0; i < int(size); i++ {
		var value int16
		err := binary.Read(buffer, binary.LittleEndian, &value)
		if err != nil {
			panic(err.Error())
		}

		meter[i] = Int(value)
	}

	return meter
}

func newPubsub() pubsub {
	return pubsub{
		pub:   make(chan Message, 10),
		sub:   make(chan chan Message, 10),
		unsub: make(chan chan Message, 10),
		st:    make(chan struct{}),
	}
}

// Start the PubSub go routine.
func (ps pubsub) start() {
	subs := make(map[chan Message]struct{})
	for {
		select {
		case sub := <-ps.sub:
			subs[sub] = struct{}{}
		case sub := <-ps.unsub:
			delete(subs, sub)
			close(sub)
		case msg := <-ps.pub:
			for sub := range subs {
				sub <- msg
			}
		case <-ps.st:
			close(ps.pub)
			close(ps.sub)
			return
		}
	}
}

func (ps pubsub) publish(msg Message) {
	ps.pub <- msg
}

func (ps pubsub) subscribe() chan Message {
	ch := make(chan Message, 10)
	ps.sub <- ch
	return ch
}

func (ps pubsub) unsubscribe(ch chan Message) {
	ps.unsub <- ch
}

func (ps pubsub) stop() {
	close(ps.st)
}
