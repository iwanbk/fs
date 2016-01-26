package tracker

import (
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/op/go-logging"
	"time"
)

var (
	FileTimeout = 15 * time.Minute
)

var (
	log = logging.MustGetLogger("tracker")
)

type State interface {
	Ready() bool
	Touch()
	Close()
}

type Tracker interface {
	Touch(name string)
	Forget(name string)
	Close(name string)
	IterReady() <-chan string
}

type trackerImpl struct {
	cache map[string]State
}

func NewTracker() Tracker {
	return &trackerImpl{
		cache: make(map[string]State),
	}
}

func (t *trackerImpl) get(name string) State {
	if state, ok := t.cache[name]; !ok {
		state = newState()
		t.cache[name] = state
		return state
	} else {
		return state
	}
}

func (t *trackerImpl) Touch(name string) {
	log.Debugf("Touching file '%s'", name)
	t.get(name).Touch()
}

func (t *trackerImpl) Forget(name string) {
	delete(t.cache, name)
}

func (t *trackerImpl) Close(name string) {
	log.Debugf("Closing file '%s'", name)
	t.get(name).Close()
}

func (t *trackerImpl) IterReady() <-chan string {
	ch := make(chan string)

	go func(ch chan string) {
		defer close(ch)
		for path, state := range t.cache {
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

type metaPurgeTracker struct{}

//NewPurgeTracker creates a tracker that mark file changes by deleting the meta file.
func NewPurgeTracker() Tracker {
	return &metaPurgeTracker{}
}

func (t *metaPurgeTracker) Touch(name string) {
	m := meta.GetMeta(name)
	if !m.Exists() {
		//create an empty meta file.
		m.Save(&meta.MetaFile{})
	}

	stat := m.Stat()
	stat = stat.SetModified(true)
	stat = stat.SetDeleted(false)
	m.SetStat(stat)
}

func (t *metaPurgeTracker) Forget(name string) {

}

func (t *metaPurgeTracker) Close(name string) {

}

func (t *metaPurgeTracker) IterReady() <-chan string {
	return nil
}
