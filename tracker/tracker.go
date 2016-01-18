package tracker

import (
	"github.com/op/go-logging"
	"time"
)

const (
	FileTimeout = 15 * time.Minute
)

var (
	log   = logging.MustGetLogger("tracker")
	cache map[string]State
)

func init() {
	cache = make(map[string]State)
}

type State interface {
	Ready() bool
	Touch()
	Close()
}

func get(name string) State {
	if state, ok := cache[name]; !ok {
		state = newState()
		cache[name] = state
		return state
	} else {
		return state
	}
}

func Touch(name string) {
	log.Debugf("Touching file '%s'", name)
	get(name).Touch()
}

func Ready(name string) bool {
	return get(name).Ready()
}

func Forget(name string) {
	delete(cache, name)
}

func Close(name string) {
	log.Debugf("Closing file '%s'", name)
	get(name).Close()
}

func IterReady() <-chan string {
	ch := make(chan string)

	go func(ch chan string) {
		defer close(ch)
		for path, state := range cache {
			if state.Ready() {
				ch <- path
			}
		}
	}(ch)

	return ch
}

type state struct {
	time   time.Time
	closed bool
}

func newState() State {
	return &state{
		time: time.Now(),
	}
}

func (s *state) Touch() {
	s.time = time.Now()
}

func (s *state) Close() {
	s.closed = true
}

func (s *state) Ready() bool {
	return s.closed || time.Now().Sub(s.time) > FileTimeout
}
