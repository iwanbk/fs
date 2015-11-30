package cache

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"io"
	"fmt"
	"bytes"
)

var (
	meta = []string {
		"/opt/mongodb/bin/mongod|d7ca41fbf8cb8a03fc70d773c32ec8d2|23605576",
		"/opt/mongodb/bin/mongos|8e7100afca707b38c1d438a4be48d0b2|18354848",
		"/opt/mongodb/bin/mongo|71ae6457a07eb4cc69bead58e497cb07|11875136",
	}
)
func TestChroot(t *testing.T) {
	tt := []struct {
		base   string
		path   string
		expect string
	}{
		{
			"/mnt",
			"/opt/code",
			"/mnt/opt/code",
		},
		{
			"/mnt",
			"opt/code",
			"/mnt/opt/code",
		},
	}

	for _, test := range tt {
		assert.Equal(t, test.expect, chroot(test.base, test.path))
	}
}

type mockCache struct {
	mock.Mock
	name string
}


//type Cache interface {
//	Open(path string) (io.ReadSeeker, error)
//	GetMetaData(id string) ([]string, error)
//	Exists(path string) bool
//	BasePath() string
//}

func (c *mockCache) String() string {
	return fmt.Sprintf("Mock cache '%s'", c.name)
}

func (c *mockCache) Open(path string) (io.ReadSeeker, error) {
	args := c.Called(path)
	if t, ok := args.Get(0).(io.ReadSeeker); ok {
		return t, nil
	}

	return nil, args.Error(1)
}

func (c *mockCache) GetMetaData(id string) ([]string, error) {
	args := c.Called(id)
	return args.Get(0).([]string), args.Error(1)
}

func (c *mockCache) Exists(path string) bool {
	args := c.Called(path)
	return args.Bool(0)
}

func (c *mockCache) BasePath() string {
	args := c.Called()
	return args.String(0)
}

type writableCache struct {
	mockCache
}

//type CacheWriter interface {
//	SetMetaData([]string) error
//	DeDupe(string, io.ReadSeeker) error
//}

func (c *writableCache) DeDupe(path string, file io.ReadSeeker) error {
	args := c.Called(path, file)
	return args.Error(0)
}

func (c *writableCache) SetMetaData(meta []string) error {
	args := c.Called(meta)
	return args.Error(0)
}

func TestCacheManagerOpenError(t *testing.T) {
	manager := NewCacheManager()
	cache := &mockCache{
		name: "1",
	}

	err := fmt.Errorf("Not found")
	path := "/path/to/file"
	cache.On("Open", path).Return(nil, err)
	manager.AddLayer(cache)

	f, e := manager.Open(path)
	assert.Error(t, e)
	assert.Nil(t, f)
}

func TestCacheManagerOpen(t *testing.T) {
	manager := NewCacheManager()
	cache1 := &mockCache{
		name: "1",
	}

	cache2 := &mockCache{
		name: "2",
	}

	err := fmt.Errorf("Not found")
	path := "/path/to/file"
	cache1.On("Open", path).Return(nil, err)
	cache2.On("Open", path).Return(bytes.NewReader([]byte("Hello World")), nil)
	manager.AddLayer(cache1)
	manager.AddLayer(cache2)

	f, e := manager.Open(path)
	assert.Nil(t, e)
	assert.NotNil(t, f)
	assert.Implements(t, (*io.ReadSeeker)(nil), f)
}

func TestCacheManagerDeDupe(t *testing.T) {
	manager := NewCacheManager()
	cache1 := &writableCache{
		mockCache: mockCache{
			name: "1",
		},
	}
	cache2 := &mockCache{
		name: "2",
	}

	err := fmt.Errorf("Not found")
	path := "/path/to/file"

	file := bytes.NewReader([]byte("Hello World"))

	cache1.
		On("Open", path).Return(nil, err).
		On("DeDupe", path, file).Return(nil)

	assert.Implements(t, (*CacheWriter)(nil), cache1)

	cache2.
		On("Open", path).Return(file, nil)

	manager.AddLayer(cache1)
	manager.AddLayer(cache2)

	f, e := manager.Open(path)
	assert.Nil(t, e)
	assert.NotNil(t, f)
	assert.Implements(t, (*io.ReadSeeker)(nil), f)

	if f, ok := f.(io.Closer); ok {
		f.Close()
	} else {
		t.Error("Can't close stream")
	}

}