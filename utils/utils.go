package utils

import (
	"github.com/op/go-logging"
	"os"
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

func Exists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}

	return !os.IsNotExist(err)
}

func IsWritable(name string) bool {
	stat, err := os.Stat(name)
	if err != nil {
		//on error we assume file doesn't exist and it
		//can be written
		return true
	}

	perm := stat.Mode().Perm()
	return perm&0222 != 0
}

//TODO implement an ExistsAndWritable call
