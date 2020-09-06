package main

import (
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
func (xair XAir) Start() {
	defer Log.Info.Printf("connection to %s closed\n", xair.name)

	go xair.ps.start()
	defer xair.ps.stop()

	updateSub := xair.Subscribe()
	go xair.update(updateSub)
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
	defer Log.Debug.Printf("%s refresh goroutine terminated\n", xair.name)

	for {
		xair.Send(Message{Address: "/xinfo"})
		xair.Send(Message{Address: "/xremote"})
		for _, meter := range xair.meters {
			xair.Send(Message{Address: "/meters", Arguments: Arguments{meter}})
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (xair XAir) update(sub chan Message) {
	defer Log.Debug.Printf("%s update goroutine terminated\n", xair.name)

	for msg := range sub {
		if !strings.HasPrefix(msg.Address, "/meters/") {
			if msg.Address == "/xinfo" {
				Log.Debug.Printf("received from %s: %s\n", xair.name, msg)
				xair.name = msg.Arguments[1].String()
			} else {
				Log.Info.Printf("received from %s: %s\n", xair.name, msg)
			}

			xair.cache[msg.Address] = msg
		}
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xair XAir) Close() {
	Log.Debug.Printf("closing connection to %s", xair.name)
	xair.conn.Close()
}

// Subscribe to messages received from the XAir device.
func (xair XAir) Subscribe() chan Message {
	ch := xair.ps.subscribe()
	Log.Debug.Printf("subscribing %p to %s", ch, xair.name)
	return ch
}

// Unsubscribe from messages from the XAir device.
func (xair XAir) Unsubscribe(ch chan Message) {
	Log.Debug.Printf("unsubscribing %p from %s", ch, xair.name)
	xair.ps.unsubscribe(ch)
}

// Get the value of an address from the XAir device.
func (xair XAir) Get(address string) (Message, error) {
	if msg, ok := xair.cache[address]; ok {
		return msg, nil
	}
	sub := xair.Subscribe()
	defer xair.Unsubscribe(sub)
	xair.Send(Message{Address: address})
	for {
		select {
		case msg := <-sub:
			if msg.Address == address {
				return msg, nil
			}
		case <-time.After(1 * time.Second):
			return Message{}, ErrTimeout
		}
	}
}

// Send a message to the XAir device.
func (xair XAir) Send(msg Message) {
	Log.Debug.Printf("sending to %s: %s\n", xair.name, msg)
	_, err := xair.conn.Write(msg.Bytes())
	if err != nil {
		Log.Warn.Printf("cannot send to %s because connection is closed\n", xair.name)
	}
}

// Receive a message from the XAir device.
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

	return msg, nil
}

func newPubsub() pubsub {
	return pubsub{
		pub:   make(chan Message),
		sub:   make(chan chan Message),
		unsub: make(chan chan Message),
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
		case msg := <-ps.pub:
			for sub := range subs {
				sub <- msg
			}
		case <-ps.st:
			for sub := range subs {
				close(sub)
			}
			close(ps.pub)
			close(ps.sub)
			close(ps.unsub)
			return
		}
	}
}

func (ps pubsub) publish(msg Message) {
	ps.pub <- msg
}

func (ps pubsub) subscribe() chan Message {
	ch := make(chan Message)
	ps.sub <- ch
	return ch
}

func (ps pubsub) unsubscribe(ch chan Message) {
	ps.unsub <- ch
	close(ch)
}

func (ps pubsub) stop() {
	close(ps.st)
}
