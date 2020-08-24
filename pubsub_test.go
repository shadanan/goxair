package goxair

import (
	"testing"
)

func TestTwoSubscribers(t *testing.T) {
	ps := NewPubSub()
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()

	ps.Publish("hello")

	if ans := <-sub1; ans != "hello" {
		t.Errorf("<-sub1 = %s; want hello", ans)
	}

	if ans := <-sub2; ans != "hello" {
		t.Errorf("<-sub1 = %s; want hello", ans)
	}
}

func TestZeroSubscribers(t *testing.T) {
	ps := NewPubSub()
	go ps.Start()
	defer ps.Stop()
	ps.Publish("hello")
}

func TestUnsubscribe(t *testing.T) {
	ps := NewPubSub()
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()
	ps.Unsubscribe(sub2)

	ps.Publish("hello")

	if ans := <-sub1; ans != "hello" {
		t.Errorf("<-sub1 = %s; want hello", ans)
	}

	_, ok := <-sub2
	if ok {
		t.Errorf("want sub2 to be closed")
	}
}

func TestStop(t *testing.T) {
	ps := NewPubSub()
	go ps.Start()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()
	ps.Stop()

	if _, ok := <-sub1; ok {
		t.Errorf("want sub1 to be closed")
	}

	if _, ok := <-sub2; ok {
		t.Errorf("want sub2 to be closed")
	}
}
