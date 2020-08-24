package goxair

// PubSub struct
type PubSub struct {
	pub   chan interface{}
	sub   chan chan interface{}
	unsub chan chan interface{}
	stop  chan struct{}
}

// NewPubSub creates a new Pubsub. Run Start() in a go routine.
func NewPubSub() PubSub {
	return PubSub{
		pub:   make(chan interface{}),
		sub:   make(chan chan interface{}),
		unsub: make(chan chan interface{}),
		stop:  make(chan struct{}),
	}
}

// Start the PubSub go routine.
func (ps PubSub) Start() {
	subs := map[chan interface{}]struct{}{}
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
		case <-ps.stop:
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

// Publish a message.
func (ps PubSub) Publish(msg interface{}) {
	ps.pub <- msg
}

// Subscribe to the PubSub.
func (ps PubSub) Subscribe() chan interface{} {
	ch := make(chan interface{})
	ps.sub <- ch
	return ch
}

// Unsubscribe from the PubSub.
func (ps PubSub) Unsubscribe(ch chan interface{}) {
	ps.unsub <- ch
	close(ch)
}

// Stop the PubSub routine.
func (ps PubSub) Stop() {
	close(ps.stop)
}
