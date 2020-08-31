package main

import (
	"errors"
	"net"
	"time"
)

// XAir client
type XAir struct {
	conn *net.UDPConn
	ps   pubsub
}

type pubsub struct {
	pub   chan Message
	sub   chan chan Message
	unsub chan chan Message
	st    chan struct{}
}

// NewXAir creates a new XAir client.
func NewXAir(address string) XAir {
	raddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		panic(err.Error())
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		panic(err.Error())
	}

	ps := newPubsub()
	return XAir{conn, ps}
}

// Start polling for messages and publish when they are received.
func (xair XAir) Start() {
	go xair.ps.start()
	defer xair.ps.stop()

	for {
		msg, err := xair.receive()
		if errors.Is(err, ErrConnClosed) {
			return
		}
		xair.ps.publish(msg)
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xair XAir) Close() {
	xair.conn.Close()
}

// Subscribe to messages received from the XAir device.
func (xair XAir) Subscribe() chan Message {
	return xair.ps.subscribe()
}

// Unsubscribe from messages from the XAir device.
func (xair XAir) Unsubscribe(ch chan Message) {
	xair.ps.unsubscribe(ch)
}

// Get the value of an address from the XAir device.
func (xair XAir) Get(address string) (Message, error) {
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
	_, err := xair.conn.Write(msg.Bytes())
	if err != nil {
		panic(err.Error())
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
	subs := map[chan Message]struct{}{}
	for {
		select {
		case msg := <-ps.sub:
			subs[msg] = struct{}{}
		case msg := <-ps.unsub:
			delete(subs, msg)
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
