package xair

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/shadanan/goxair/log"
	"github.com/shadanan/goxair/osc"
	"github.com/shadanan/goxair/pubsub"
)

// XAir client
type XAir struct {
	Name   string
	conn   *net.UDPConn
	ps     pubsub.Message
	get    chan cacheReq
	set    chan osc.Message
	stop   chan struct{}
	meters osc.Arguments
}

type cacheReq struct {
	address string
	ch      chan cacheResp
}

type cacheResp struct {
	msg osc.Message
	ok  bool
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

	conn.SetReadBuffer(1048576)
	conn.SetWriteBuffer(1048576)

	meterArgs := make([]osc.Argument, len(meters))
	for i, meter := range meters {
		meterArgs[i] = osc.String(fmt.Sprintf("/meters/%d", meter))
	}

	return XAir{
		Name:   name,
		conn:   conn,
		ps:     pubsub.NewMessage(10),
		get:    make(chan cacheReq, 10),
		set:    make(chan osc.Message, 10),
		stop:   make(chan struct{}),
		meters: meterArgs,
	}
}

// Start polling for messages and publish when they are received.
func (xa XAir) Start(terminate chan string, timeout time.Duration) {
	go xa.ps.Start()
	defer xa.ps.Stop()

	sub := xa.Subscribe()
	go xa.monitor(sub, terminate, timeout)
	defer xa.Unsubscribe(sub)

	stopCacher := make(chan struct{})
	go xa.cacher(stopCacher)
	defer close(stopCacher)

	go xa.refresher()

	for {
		msg, err := xa.receive()
		if err != nil {
			log.Info.Printf("Connection to %s closed.", xa.Name)
			return
		}
		xa.ps.Publish(msg)
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xa XAir) Close() {
	close(xa.stop)
}

// Subscribe to messages received from the XAir device.
func (xa XAir) Subscribe() chan osc.Message {
	ch := xa.ps.Subscribe()
	log.Debug.Printf("Subscribed %p to %s.", ch, xa.Name)
	return ch
}

// Unsubscribe from messages from the XAir device.
func (xa XAir) Unsubscribe(ch chan osc.Message) {
	xa.ps.Unsubscribe(ch)
	log.Debug.Printf("Unsubscribed %p from %s.", ch, xa.Name)
}

// Get the value of an address on the XAir device.
func (xa XAir) Get(address string) (osc.Message, bool, error) {
	ch := make(chan cacheResp)
	xa.get <- cacheReq{address, ch}
	resp := <-ch
	if resp.ok {
		log.Debug.Printf("Get on %s (cached): %s", xa.Name, resp.msg)
		return resp.msg, true, nil
	}

	sub := xa.Subscribe()
	defer xa.Unsubscribe(sub)

	retry := 0
	timeout := time.NewTimer(1 * time.Second)
	xa.send(osc.Message{Address: address})

	for {
		select {
		case msg := <-sub:
			if msg.Address == address {
				log.Debug.Printf("Get on %s: %s", xa.Name, msg)
				return msg, false, nil
			}
		case <-timeout.C:
			retry++
			log.Info.Printf("Get timed out %d time(s) on %s: %s",
				retry, xa.Name, address)
			if retry == 3 {
				return osc.Message{}, false, ErrTimeout
			}
			timeout.Reset(1 * time.Second)
			xa.send(osc.Message{Address: address})
		}
	}
}

// Set the value of an address on the XAir device.
func (xa XAir) Set(address string, arguments osc.Arguments) {
	msg := osc.Message{Address: address, Arguments: arguments}
	log.Info.Printf("Set on %s: %s", xa.Name, msg)
	xa.set <- msg
	xa.send(msg)
}

func (xa XAir) refresher() {
	defer log.Debug.Printf("%s refresher goroutine terminated.", xa.Name)

	for {
		xa.send(osc.Message{Address: "/xremote"})
		for _, meter := range xa.meters {
			xa.send(osc.Message{Address: "/meters", Arguments: osc.Arguments{meter}})
		}

		select {
		case <-xa.stop:
			xa.conn.Close()
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (xa XAir) monitor(sub chan osc.Message, terminate chan string, t time.Duration) {
	defer log.Debug.Printf("%s monitor goroutine terminated.", xa.Name)

	for {
		select {
		case <-time.After(t):
			log.Info.Printf("Timeout from %s.", xa.Name)
			terminate <- xa.Name
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}

			if !strings.HasPrefix(msg.Address, "/meters/") {
				log.Info.Printf("Received from %s: %s", xa.Name, msg)
				xa.set <- msg
			}
		}
	}
}

func (xa XAir) cacher(stop chan struct{}) {
	defer log.Debug.Printf("%s cacher goroutine terminated.", xa.Name)

	cache := make(map[string]osc.Message)

	for {
		select {
		case <-stop:
			return
		case req := <-xa.get:
			msg, ok := cache[req.address]
			req.ch <- cacheResp{msg, ok}
		case msg := <-xa.set:
			cache[msg.Address] = msg
		}
	}
}

func (xa XAir) send(msg osc.Message) error {
	total, err := xa.conn.Write(msg.Bytes())
	if err != nil {
		log.Error.Printf("Cannot because connection is closed.")
		return err
	}
	if total != len(msg.Bytes()) {
		log.Error.Printf("Only wrote %d of %d bytes for: %s",
			total, len(msg.Bytes()), msg)
	}
	return nil
}

func (xa XAir) receive() (osc.Message, error) {
	data := make([]byte, 65535)
	_, err := xa.conn.Read(data)
	if err != nil {
		return osc.Message{}, err
	}

	msg, err := osc.ParseMessage(data)
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

func decodeMeter(blob []byte) []osc.Argument {
	buffer := bytes.NewBuffer(blob)

	var size int32
	err := binary.Read(buffer, binary.LittleEndian, &size)
	if err != nil {
		panic(err.Error())
	}

	meter := make([]osc.Argument, size)
	for i := 0; i < int(size); i++ {
		var value int16
		err := binary.Read(buffer, binary.LittleEndian, &value)
		if err != nil {
			panic(err.Error())
		}

		meter[i] = osc.Int(value)
	}

	return meter
}
