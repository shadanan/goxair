package log

import (
	"io/ioutil"
	"log"
	"os"
)

var (
	// Debug log messages.
	Debug = log.New(ioutil.Discard, "[GOX-debug]: ", log.LstdFlags|log.Lshortfile)
	// Info log messages.
	Info = log.New(os.Stdout, "[GOX-info]: ", log.LstdFlags|log.Lshortfile)
	// Error log messages.
	Error = log.New(os.Stderr, "[GOX-error]: ", log.LstdFlags|log.Lshortfile)
)
