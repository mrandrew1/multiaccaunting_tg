package logging

import (
	"log"
	"os"
)

// NewLogger возвращает базовый логгер, пишущий в stdout.
func NewLogger() *log.Logger {
	return log.New(os.Stdout, "[pact-telegram] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
}

