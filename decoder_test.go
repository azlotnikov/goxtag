package goxtag

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestDecoder(t *testing.T) {
	asrt := assert.New(t)

	var p Page

	asrt.NoError(NewDecoder(strings.NewReader(testPage)).Decode(&p))
	asrt.Len(p.Resources, 5)
}
