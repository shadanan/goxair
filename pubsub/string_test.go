package pubsub

import (
	"reflect"
	"testing"
)

func TestStringTwoSubscribers(t *testing.T) {
	ps := NewString()
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()

	hello := "hello"
	ps.Publish(hello)
	want := hello

	if ans := <-sub1; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}

	if ans := <-sub2; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}
}

func TestStringZeroSubscribers(t *testing.T) {
	ps := NewString()
	go ps.Start()
	defer ps.Stop()

	hello := "hello"
	ps.Publish(hello)
}

func TestStringUnsubscribe(t *testing.T) {
	ps := NewString()
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()
	ps.Unsubscribe(sub2)

	hello := "hello"
	ps.Publish(hello)
	want := hello

	if ans := <-sub1; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}

	_, ok := <-sub2
	if ok {
		t.Errorf("want sub2 to be closed")
	}
}
