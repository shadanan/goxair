package xair

import (
	"bytes"
	"encoding/binary"
	"errors"
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

	ps := pubsub.NewMessage(10)
	get := make(chan cacheReq, 10)
	set := make(chan osc.Message, 10)

	meterArgs := make([]osc.Argument, len(meters))
	for i, meter := range meters {
		meterArgs[i] = osc.String(fmt.Sprintf("/meters/%d", meter))
	}

	return XAir{name, conn, ps, get, set, meterArgs}
}

// Start polling for messages and publish when they are received.
func (xa XAir) Start(terminate chan string) {
	defer log.Info.Printf("Connection to %s closed.", xa.Name)

	go xa.ps.Start()
	defer xa.ps.Stop()

	updateSub := xa.Subscribe()
	go xa.update(updateSub, terminate)
	defer xa.Unsubscribe(updateSub)

	stopRefresh := make(chan struct{})
	go xa.refresh(stopRefresh)
	defer close(stopRefresh)

	for {
		msg, err := xa.receive()
		if errors.Is(err, ErrConnClosed) {
			return
		}
		xa.ps.Publish(msg)
	}
}

func (xa XAir) refresh(stop chan struct{}) {
	defer log.Debug.Printf("%s refresh goroutine terminated.", xa.Name)

	for {
		xa.send(osc.Message{Address: "/xremote"})
		for _, meter := range xa.meters {
			xa.send(osc.Message{Address: "/meters", Arguments: osc.Arguments{meter}})
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (xa XAir) update(sub chan osc.Message, terminate chan string) {
	defer log.Debug.Printf("%s update goroutine terminated.", xa.Name)

	cache := make(map[string]osc.Message)

	for {
		select {
		case <-time.After(1 * time.Second):
			log.Info.Printf("Timeout from %s.", xa.Name)
			terminate <- xa.Name
		case msg, ok := <-sub:
			if !ok {
				return
			}

			if !strings.HasPrefix(msg.Address, "/meters/") {
				log.Info.Printf("Received from %s: %s", xa.Name, msg)
				cache[msg.Address] = msg
			}
		case req := <-xa.get:
			msg, ok := cache[req.address]
			req.ch <- cacheResp{msg, ok}
		case msg := <-xa.set:
			cache[msg.Address] = msg
		}
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xa XAir) Close() {
	xa.conn.Close()
}

// Subscribe to messages received from the XAir device.
func (xa XAir) Subscribe() chan osc.Message {
	ch := xa.ps.Subscribe()
	log.Debug.Printf("Subscribing %p to %s.", ch, xa.Name)
	return ch
}

// Unsubscribe from messages from the XAir device.
func (xa XAir) Unsubscribe(ch chan osc.Message) {
	log.Debug.Printf("Unsubscribing %p from %s.", ch, xa.Name)
	xa.ps.Unsubscribe(ch)
}

// Get the value of an address on the XAir device.
func (xa XAir) Get(address string) (osc.Message, error) {
	ch := make(chan cacheResp)
	xa.get <- cacheReq{address, ch}
	resp := <-ch
	if resp.ok {
		log.Info.Printf("Get on %s (cached): %s", xa.Name, resp.msg)
		return resp.msg, nil
	}

	sub := xa.Subscribe()
	defer xa.Unsubscribe(sub)
	timeout := time.After(1 * time.Second)
	xa.send(osc.Message{Address: address})

	for {
		select {
		case msg := <-sub:
			if msg.Address == address {
				log.Info.Printf("Get on %s: %s", xa.Name, msg)
				return msg, nil
			}
		case <-timeout:
			log.Info.Printf("Get timed out on %s: %s", xa.Name, address)
			return osc.Message{}, ErrTimeout
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

func (xa XAir) send(msg osc.Message) {
	log.Debug.Printf("Send to %s: %s", xa.Name, msg)
	_, err := xa.conn.Write(msg.Bytes())
	if err != nil {
		log.Error.Printf("Cannot send to %s because connection is closed.", xa.Name)
	}
}

func (xa XAir) receive() (osc.Message, error) {
	data := make([]byte, 65535)
	_, err := xa.conn.Read(data)
	if err != nil {
		return osc.Message{}, ErrConnClosed
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
