package xair

import "github.com/pkg/errors"

var (
	// ErrTimeout is returned when an expected message doesn't arrive in time.
	ErrTimeout = errors.New("timeout")
)
