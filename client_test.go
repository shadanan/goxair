package main

import (
	"reflect"
	"testing"
)

var helloMessage = Message{Address: "/hello"}

func TestNewXAir(t *testing.T) {
	want := "192.168.0.1:10024"
	xair := NewXAir(want)
	if xair.conn.RemoteAddr().String() != want {
		t.Errorf("%s != %s", want, xair.conn.RemoteAddr())
	}
}

func TestTwoSubscribers(t *testing.T) {
	ps := newPubsub()
	go ps.start()
	defer ps.stop()

	sub1 := ps.subscribe()
	sub2 := ps.subscribe()

	ps.publish(helloMessage)
	want := helloMessage

	if ans := <-sub1; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}

	if ans := <-sub2; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}
}

func TestZeroSubscribers(t *testing.T) {
	ps := newPubsub()
	go ps.start()
	defer ps.stop()
	ps.publish(helloMessage)
}

func TestUnsubscribe(t *testing.T) {
	ps := newPubsub()
	go ps.start()
	defer ps.stop()

	sub1 := ps.subscribe()
	sub2 := ps.subscribe()
	ps.unsubscribe(sub2)

	ps.publish(helloMessage)
	want := helloMessage

	if ans := <-sub1; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}

	_, ok := <-sub2
	if ok {
		t.Errorf("want sub2 to be closed")
	}
}

func TestStop(t *testing.T) {
	ps := newPubsub()
	go ps.start()

	sub1 := ps.subscribe()
	sub2 := ps.subscribe()
	ps.stop()

	if _, ok := <-sub1; ok {
		t.Errorf("want sub1 to be closed")
	}

	if _, ok := <-sub2; ok {
		t.Errorf("want sub2 to be closed")
	}
}
