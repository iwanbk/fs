package main

import (
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
