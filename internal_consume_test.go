package consume

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNestedMapFilter(t *testing.T) {
	assert := assert.New(t)
	var result []string
	a := AppendTo(&result)
	c := MapFilter(a, appendStr("d"), appendStr("e"))
	c = MapFilter(c, appendStr("a"), appendStr("b"), appendStr("c"))
	if c.CanConsume() {
		str := ""
		c.Consume(&str)
	}
	assert.Equal([]string{"abcde"}, result)
	mfc := c.(*mapFilterConsumer)
	assert.Same(a, mfc.Consumer)
}

func TestMapFilterWithNil(t *testing.T) {
	assert := assert.New(t)
	var ints []int
	c := AppendTo(&ints)
	assert.Same(c, MapFilter(c, NewMapFilterer()))
}

func TestComposeZero(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(nilConsumer{}, Compose())
}

func TestComposeOne(t *testing.T) {
	assert := assert.New(t)
	var ints []int
	c := AppendTo(&ints)
	assert.Same(c, Compose(c))
}

func appendStr(s string) func(src, dest *string) bool {
	return func(src, dest *string) bool {
		*dest = *src + s
		return true
	}
}
