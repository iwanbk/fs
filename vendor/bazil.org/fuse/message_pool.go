package fuse

var (
	mPool *messagePool
)

type messagePool struct {
	c    chan *message
	gorC chan *message
}

func init() {
	mPool = newMessagePool(10)
}
func newMessagePool(size int) *messagePool {
	mp := messagePool{
		c:    make(chan *message, size),
		gorC: make(chan *message, 3),
	}
	for i := 0; i < size; i++ {
		mp.c <- allocMessage().(*message)
	}
	go func() {
		for {
			mp.gorC <- allocMessage().(*message)
		}
	}()
	return &mp
}
func (mp *messagePool) Get() (m *message) {
	select {
	case m = <-mp.c:
	default:
		m = <-mp.gorC
	}
	return
}

func (mp *messagePool) Put(m *message) {
	select {
	case mp.c <- m:
	default:
		m.buf = nil
	}
}
