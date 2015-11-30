package filesystem

import (
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestFileBuffer(t *testing.T) {
	buffer := NewFileBuffer(nil).(*fileBufferImpl)

	assert.Len(t, buffer.buffer, FileReadBuffer)
	assert.False(t, buffer.available(0, 100))

	buffer.size = 100
	assert.True(t, buffer.available(0, 100))

	buffer.offset = 100
	buffer.size = 200

	assert.False(t, buffer.available(0, 150))
	assert.False(t, buffer.available(100, 250))
	assert.True(t, buffer.available(100, 200))
	assert.True(t, buffer.available(150, 50))
	assert.True(t, buffer.available(150, 150))
	assert.True(t, buffer.available(200, 100))
}
