package watcher

import (
	"fmt"
	"gopkg.in/natefinch/lumberjack.v2"
	logging "log"
	"time"
)

type TLogger interface {
	Log(path, hash string)
}

type tlogger struct {
	logger *logging.Logger
}

func NewTLogger(name string) TLogger {
	logger := logging.New(
		&lumberjack.Logger{
			Filename:   name,
			MaxSize:    5, //Megabytes
			MaxBackups: 1,
			MaxAge:     1, //Days
			LocalTime:  true,
		},
		"", 0)

	return &tlogger{
		logger: logger,
	}
}

func (l *tlogger) Log(path, hash string) {
	l.logger.Println(fmt.Sprintf("%s|%s|%d", path, hash, time.Now().Unix()))
}
