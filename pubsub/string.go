package pubsub

// String is a pubsub for strings.
type String struct {
	pub   chan string
	sub   chan chan string
	unsub chan chan string
	st    chan struct{}
}

// NewString creates a new String pubsub.
func NewString() String {
	return String{
		pub:   make(chan string),
		sub:   make(chan chan string),
		unsub: make(chan chan string),
		st:    make(chan struct{}),
	}
}

// Start the pubsub goroutine.
func (ps String) Start() {
	subs := make(map[chan string]bool)

	for {
		select {
		case msg := <-ps.pub:
			for sub := range subs {
				sub <- msg
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

// Publish a message to all subscribers.
func (ps String) Publish(msg string) {
	ps.pub <- msg
}

// Subscribe to updates.
func (ps String) Subscribe() chan string {
	sub := make(chan string, 10)
	ps.sub <- sub
	return sub
}

// Unsubscribe from updates.
func (ps String) Unsubscribe(sub chan string) {
	ps.unsub <- sub
}

// Stop the pubsub goroutine.
func (ps String) Stop() {
	close(ps.st)
}
