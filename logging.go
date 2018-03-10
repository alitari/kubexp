package kubeExplorer

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var (
	tracelog       *log.Logger
	infolog        *log.Logger
	warninglog     *log.Logger
	errorlog       *log.Logger
	fatalStderrlog *log.Logger
	warnStderrlog  *log.Logger
)

func initLog(writer io.Writer) {
	fatalStderrlog = log.New(os.Stderr,
		"FATAL: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	warnStderrlog = log.New(os.Stderr,
		"WARN: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	traceWriter := ioutil.Discard
	infoWriter := ioutil.Discard
	warningWriter := ioutil.Discard
	errorWriter := ioutil.Discard

	switch strings.ToLower(*logLevel) {
	case "trace":
		traceWriter = writer
		fallthrough
	case "info":
		infoWriter = writer
		fallthrough
	case "warn":
		warningWriter = writer
		fallthrough
	case "error":
		errorWriter = writer
	default:
		fatalStderrlog.Panicf("Loglevel: '%s' not supported!", *logLevel)
	}

	tracelog = log.New(traceWriter,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	infolog = log.New(infoWriter,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	warninglog = log.New(warningWriter,
		"WARN: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	errorlog = log.New(errorWriter,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	//defer logFile.Close()
}
