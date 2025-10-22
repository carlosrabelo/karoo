package logger

import (
	"log"
	"os"
)

type Logger struct {
	info  *log.Logger
	error *log.Logger
	debug *log.Logger
}

var Default = New()

func New() *Logger {
	return &Logger{
		info:  log.New(os.Stdout, "[INFO] ", log.LstdFlags),
		error: log.New(os.Stderr, "[ERROR] ", log.LstdFlags),
		debug: log.New(os.Stdout, "[DEBUG] ", log.LstdFlags),
	}
}

func (l *Logger) Info(format string, v ...any) {
	l.info.Printf(format, v...)
}

func (l *Logger) Error(format string, v ...any) {
	l.error.Printf(format, v...)
}

func (l *Logger) Debug(format string, v ...any) {
	l.debug.Printf(format, v...)
}

func Info(format string, v ...any) {
	Default.Info(format, v...)
}

func Error(format string, v ...any) {
	Default.Error(format, v...)
}

func Debug(format string, v ...any) {
	Default.Debug(format, v...)
}
