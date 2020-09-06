package main

import (
	"log"
	"os"
)

var (
	// Log contains loggers for Debug, Info, and Error.
	Log = logger{
		Debug: log.New(os.Stdout, "[D]: ", log.LstdFlags|log.Lshortfile),
		Info:  log.New(os.Stdout, "[I]: ", log.LstdFlags|log.Lshortfile),
		Warn:  log.New(os.Stderr, "[W]: ", log.LstdFlags|log.Lshortfile),
		Error: log.New(os.Stderr, "[E]: ", log.LstdFlags|log.Lshortfile),
	}
)

type logger struct {
	Debug *log.Logger
	Info  *log.Logger
	Warn  *log.Logger
	Error *log.Logger
}
