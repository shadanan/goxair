package pubsub

import (
	"reflect"
	"testing"

	"github.com/shadanan/goxair/osc"
)

func TestMessageTwoSubscribers(t *testing.T) {
	ps := NewMessage(0)
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()

	hello := osc.Message{Address: "/hello"}
	ps.Publish(hello)
	want := hello

	if ans := <-sub1; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}

	if ans := <-sub2; !reflect.DeepEqual(ans, want) {
		t.Errorf("<-sub1 = %s; want %s", ans, want)
	}
}

func TestMessageZeroSubscribers(t *testing.T) {
	ps := NewMessage(0)
	go ps.Start()
	defer ps.Stop()

	hello := osc.Message{Address: "/hello"}
	ps.Publish(hello)
}

func TestMessageUnsubscribe(t *testing.T) {
	ps := NewMessage(0)
	go ps.Start()
	defer ps.Stop()

	sub1 := ps.Subscribe()
	sub2 := ps.Subscribe()
	ps.Unsubscribe(sub2)

	hello := osc.Message{Address: "/hello"}
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
