package utils

import (
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("utils")
)

func In(l []string, x string) bool {
	for _, y := range l {
		if x == y {
			return true
		}
	}

	return false
}