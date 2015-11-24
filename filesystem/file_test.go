package filesystem

import (
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestNewFileInfo(t *testing.T) {
	fi, err := newFileInfo("arakoonStart.py|a0acf38b3f672a42ed177e9ed09eec3f|915")
	assert.NoError(t, err)
	assert.Equal(t, "arakoonStart.py", fi.Filename)
	assert.EqualValues(t, 915, fi.Size)
	assert.Equal(t, "a0acf38b3f672a42ed177e9ed09eec3f", fi.Hash)
}
