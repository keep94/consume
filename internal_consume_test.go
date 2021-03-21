package consume

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestMapFilterWithNil(t *testing.T) {
	assert := assert.New(t)
	var ints []int
	c := AppendTo(&ints)
	assert.Same(c, MapFilter(c, NewMapFilterer()))
}

func TestNilMapFilterer(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(nilMapFilterer{}, NewMapFilterer())
}

func TestSingleMapFilterer(t *testing.T) {
	assert := assert.New(t)
	filter := NewMapFilterer(
		func(ptr *int) bool {
			return (*ptr)%2 == 0
		})
	// filters are already thred-safe so no cloning necessary
	assert.Same(filter, NewMapFilterer(filter))
}

func TestSingleMapper(t *testing.T) {
	assert := assert.New(t)
	mapper := NewMapFilterer(
		func(src *int, dest *string) bool {
			*dest = strconv.Itoa(*src)
			return true
		})
	mapperClone := NewMapFilterer(mapper)
	assert.NotSame(mapper, mapperClone)
	i := 25
	twentyFiveStrPtr := mapper.MapFilter(&i).(*string)
	i = 43
	fortyThreeStrPtr := mapperClone.MapFilter(&i).(*string)
	assert.Equal("25", *twentyFiveStrPtr)
	assert.Equal("43", *fortyThreeStrPtr)
}

func TestMapFiltererIndependence(t *testing.T) {
	assert := assert.New(t)
	toString := NewMapFilterer(
		func(src *int, dest *string) bool {
			*dest = strconv.Itoa(*src)
			return true
		})
	evenSquarePlus1 := NewMapFilterer(
		func(ptr *int) bool {
			return *ptr%2 == 0
		},
		NewMapFilterer(),
		func(src, dest *int) bool {
			*dest = (*src) * (*src)
			return true
		},
		func(src, dest *int) bool {
			*dest = (*src) + 1
			return true
		})
	i := 7
	assert.Nil(evenSquarePlus1.MapFilter(&i))
	i = 8
	sixtyFivePtr := evenSquarePlus1.MapFilter(&i).(*int)
	i = 43
	fortyThreeStrPtr := toString.MapFilter(&i).(*string)

	evenSquarePlus1Str := NewMapFilterer(evenSquarePlus1, toString)
	i = 10
	oneHundredOneStrPtr := evenSquarePlus1Str.MapFilter(&i).(*string)

	assert.Equal(1, toString.size())
	assert.Equal(3, evenSquarePlus1.size())
	assert.Equal(4, evenSquarePlus1Str.size())

	assert.Equal(65, *sixtyFivePtr)
	assert.Equal("43", *fortyThreeStrPtr)
	assert.Equal("101", *oneHundredOneStrPtr)
}
