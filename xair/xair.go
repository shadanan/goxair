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
	cache  map[string]osc.Message
	meters osc.Arguments
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
	cache := make(map[string]osc.Message)

	meterArgs := make([]osc.Argument, len(meters))
	for i, meter := range meters {
		meterArgs[i] = osc.String(fmt.Sprintf("/meters/%d", meter))
	}

	return XAir{name, conn, ps, cache, meterArgs}
}

// Start polling for messages and publish when they are received.
func (xair XAir) Start(terminate chan string) {
	defer log.Info.Printf("Connection to %s closed.", xair.Name)

	go xair.ps.Start()
	defer xair.ps.Stop()

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
		xair.ps.Publish(msg)
	}
}

func (xair XAir) refresh(stop chan struct{}) {
	defer log.Debug.Printf("%s refresh goroutine terminated.", xair.Name)

	for {
		xair.send(osc.Message{Address: "/xremote"})
		for _, meter := range xair.meters {
			xair.send(osc.Message{Address: "/meters", Arguments: osc.Arguments{meter}})
		}

		select {
		case <-stop:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (xair XAir) update(sub chan osc.Message, terminate chan string) {
	defer log.Debug.Printf("%s update goroutine terminated.", xair.Name)

	timeout := time.NewTicker(1 * time.Second)
	for {
		select {
		case v := <-timeout.C:
			log.Info.Printf("Timeout from %s: %s.", xair.Name, v)
			terminate <- xair.Name
		case msg, ok := <-sub:
			if !ok {
				return
			}

			timeout.Reset(1 * time.Second)
			if !strings.HasPrefix(msg.Address, "/meters/") {
				log.Info.Printf("Received from %s: %s", xair.Name, msg)
				xair.cache[msg.Address] = msg
			}
		}
	}
}

// Close the connection to the XAir and shutdown all channels.
func (xair XAir) Close() {
	xair.conn.Close()
}

// Subscribe to messages received from the XAir device.
func (xair XAir) Subscribe() chan osc.Message {
	ch := xair.ps.Subscribe()
	log.Debug.Printf("Subscribing %p to %s.", ch, xair.Name)
	return ch
}

// Unsubscribe from messages from the XAir device.
func (xair XAir) Unsubscribe(ch chan osc.Message) {
	log.Debug.Printf("Unsubscribing %p from %s.", ch, xair.Name)
	xair.ps.Unsubscribe(ch)
}

// Get the value of an address on the XAir device.
func (xair XAir) Get(address string) (osc.Message, error) {
	if msg, ok := xair.cache[address]; ok {
		log.Info.Printf("Get on %s (cached): %s", xair.Name, msg)
		return msg, nil
	}
	sub := xair.Subscribe()
	defer xair.Unsubscribe(sub)
	xair.send(osc.Message{Address: address})
	for {
		select {
		case msg := <-sub:
			if msg.Address == address {
				log.Info.Printf("Get on %s: %s", xair.Name, msg)
				return msg, nil
			}
		case <-time.After(1 * time.Second):
			log.Info.Printf("Get timed out on %s: %s", xair.Name, address)
			return osc.Message{}, ErrTimeout
		}
	}
}

// Set the value of an address on the XAir device.
func (xair XAir) Set(address string, arguments osc.Arguments) {
	msg := osc.Message{Address: address, Arguments: arguments}
	log.Info.Printf("Set on %s: %s", xair.Name, msg)
	xair.cache[msg.Address] = msg
	xair.send(msg)
}

func (xair XAir) send(msg osc.Message) {
	log.Debug.Printf("Send to %s: %s", xair.Name, msg)
	_, err := xair.conn.Write(msg.Bytes())
	if err != nil {
		log.Error.Printf("Cannot send to %s because connection is closed.", xair.Name)
	}
}

func (xair XAir) receive() (osc.Message, error) {
	data := make([]byte, 65535)
	_, err := xair.conn.Read(data)
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
