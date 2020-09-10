package main

import (
	"io/ioutil"
	"log"
	"os"
)

var (
	// Log contains loggers for Debug, Info, and Error.
	Log = logger{
		Debug: log.New(ioutil.Discard, "[GOX-debug]: ", log.LstdFlags|log.Lshortfile),
		Info:  log.New(os.Stdout, "[GOX-info]: ", log.LstdFlags|log.Lshortfile),
		Error: log.New(os.Stderr, "[GOX-error]: ", log.LstdFlags|log.Lshortfile),
	}
)

type logger struct {
	Debug *log.Logger
	Info  *log.Logger
	Error *log.Logger
}
