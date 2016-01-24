package meta

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMetaStatInitial(t *testing.T) {
	s := MetaState(0000)

	assert.False(t, s.Modified())
	assert.False(t, s.Deleted())
}

func TestMetaStatMod(t *testing.T) {
	s := MetaState(0200)

	assert.True(t, s.Modified())
	assert.False(t, s.Deleted())
}

func TestMetaStatDel(t *testing.T) {
	s := MetaState(0100)

	assert.False(t, s.Modified())
	assert.True(t, s.Deleted())
}

func TestSetMetaStatMod(t *testing.T) {
	s := MetaState(0000)

	assert.False(t, s.Modified())

	m := s.SetModified(true)
	assert.True(t, m.Modified())

	m = s.SetModified(false)
	assert.False(t, m.Modified())
}

func TestSetMetaStatDel(t *testing.T) {
	s := MetaState(0000)

	assert.False(t, s.Deleted())

	m := s.SetDeleted(true)
	assert.True(t, m.Deleted())

	m = s.SetDeleted(false)
	assert.False(t, m.Deleted())
}
