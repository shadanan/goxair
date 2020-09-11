package xair

import (
	"testing"
)

func TestNewXAir(t *testing.T) {
	want := "192.168.0.1:10024"
	xair := NewXAir(want, "", []int{})
	if xair.conn.RemoteAddr().String() != want {
		t.Errorf("%s != %s", want, xair.conn.RemoteAddr())
	}
}
