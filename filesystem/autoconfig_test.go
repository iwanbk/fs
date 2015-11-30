package filesystem

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jumpscale/aysfs/cache"
	"github.com/stretchr/testify/assert"
)

func TestTestListen(t *testing.T) {
	l, err := net.Listen("tcp", ":8000")
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				t.Errorf("error accepting connection: %v\n", err)
				return
			}
			go func(conn net.Conn) {
				time.Sleep(time.Second)
				conn.Close()
			}(conn)
		}
	}()
	err = testListen("localhost:8000")
	assert.NoError(t, err)

	err = testListen("localhost:9009")
	assert.Error(t, err)
}

func TestDiscoverMetadata(t *testing.T) {
	fixtures := `/opt/mongodb/bin/mongod|d7ca41fbf8cb8a03fc70d773c32ec8d2|23605576
/opt/mongodb/bin/mongos|8e7100afca707b38c1d438a4be48d0b2|18354848
/opt/mongodb/bin/mongo|71ae6457a07eb4cc69bead58e497cb07|11875136`

	dir, err := ioutil.TempDir("", "aysfs_test")
	defer func() { os.Remove(dir) }()
	if err != nil {
		t.Errorf("error while creating temp dir: %v\n", err)
		return
	}
	ioutil.WriteFile(filepath.Join(dir, "jumpscale__mongodb.flist"), []byte(fixtures), 0770)
	ioutil.WriteFile(filepath.Join(dir, "notValid"), []byte("/dir/helloworld"), 0770)

	fs := NewFS("/", cache.NewCacheManager())
	err = discoverMetadata(fs, dir)
	assert.NoError(t, err)

	node := fs.metadata.Search("opt/mongodb/bin")
	if !assert.NotNil(t, node) {
		t.FailNow()
	}
	assert.False(t, node.IsLeaf())
	assert.Equal(t, 3, len(node.Children()))

	node = fs.metadata.Search("opt/dir")
	if !assert.Nil(t, node) {
		t.FailNow()
	}
}
