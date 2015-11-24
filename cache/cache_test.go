package cache

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/stretchr/testify/assert"

	"testing"
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

func BenchmarkGetFile(b *testing.B) {
	buff := make([]byte, 10240000)
	f, err := os.Open("/dev/zero")
	if err != nil {
		b.Fatal(err)
	}
	_, err = f.Read(buff)
	if err != nil {
		b.Fatal(err)
	}

	handler := func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(buff)
	}
	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	cache := httpCache{
		addr: s.URL,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			content, err := cache.GetFileContent("/")
			if err != nil {
				b.Fatal(err)
			}
			_ = content
		}
	})
}

func BenchmarkLazyGetFile(b *testing.B) {
	buff := make([]byte, 10240000)
	f, err := os.Open("/dev/zero")
	if err != nil {
		b.Fatal(err)
	}
	_, err = f.Read(buff)
	if err != nil {
		b.Fatal(err)
	}

	handler := func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(buff)
	}
	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	cache := httpCache{
		addr: s.URL,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r, err := cache.LazyGetFileContent("/")
			if err != nil {
				b.Fatal(err)
			}

			go func() {
				content, err := ioutil.ReadAll(r)
				defer r.Close()
				if err != nil {
					b.Fatal(err)
				}
				_ = content
			}()
		}
	})
}
