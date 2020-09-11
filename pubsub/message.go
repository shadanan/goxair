package pubsub

import (
	"github.com/shadanan/goxair/osc"
)

// Message is a pubsub for messages.
type Message struct {
	pub   chan osc.Message
	sub   chan chan osc.Message
	unsub chan chan osc.Message
	st    chan struct{}
}

// NewMessage creates a new Message pubsub.
func NewMessage(buffer int) Message {
	return Message{
		pub:   make(chan osc.Message, buffer),
		sub:   make(chan chan osc.Message, buffer),
		unsub: make(chan chan osc.Message, buffer),
		st:    make(chan struct{}),
	}
}

// Start the PubSub go routine.
func (ps Message) Start() {
	subs := make(map[chan osc.Message]bool)

	for {
		select {
		case sub := <-ps.sub:
			subs[sub] = true
		case sub := <-ps.unsub:
			delete(subs, sub)
			close(sub)
		case msg := <-ps.pub:
			for sub := range subs {
				sub <- msg
			}
		case <-ps.st:
			return
		}
	}
}

// Publish a message to all subscribers.
func (ps Message) Publish(msg osc.Message) {
	ps.pub <- msg
}

// Subscribe to updates.
func (ps Message) Subscribe() chan osc.Message {
	ch := make(chan osc.Message, 10)
	ps.sub <- ch
	return ch
}

// Unsubscribe from updates.
func (ps Message) Unsubscribe(ch chan osc.Message) {
	ps.unsub <- ch
}

// Stop the pubsub goroutine.
func (ps Message) Stop() {
	close(ps.st)
}
