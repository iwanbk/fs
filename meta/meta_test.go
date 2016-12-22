package meta

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMetaStatInitial(t *testing.T) {
	s := MetaState(0000)

	assert.False(t, s.Modified())
}

func TestMetaStatMod(t *testing.T) {
	s := MetaState(MetaModified)

	assert.True(t, s.Modified())
}

func TestMetaStatDel(t *testing.T) {
	s := MetaState(MetaDeleted)

	assert.False(t, s.Modified())
}

func TestSetMetaStatMod(t *testing.T) {
	s := MetaState(0000)

	assert.False(t, s.Modified())

	m := s.SetModified(true)
	assert.True(t, m.Modified())

	m = s.SetModified(false)
	assert.False(t, m.Modified())
}
