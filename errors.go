package main

import "github.com/pkg/errors"

var (
	// ErrConnClosed is returned when the connection to XAir is closed.
	ErrConnClosed = errors.New("connection closed")
	// ErrTimeout is returned when an expected message doesn't arrive in time.
	ErrTimeout = errors.New("timeout")
)
