package consume_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/keep94/consume"
	"github.com/stretchr/testify/assert"
)

func TestNil(t *testing.T) {
	assert := assert.New(t)
	consumer := consume.Nil()
	assert.False(consumer.CanConsume())
	assert.Panics(func() { consumer.Consume(new(int)) })
}

func TestPageConsumer(t *testing.T) {
	assert := assert.New(t)
	var arr []int
	var morePages bool
	pager := consume.Page(0, 5, &arr, &morePages)
	feedInts(t, pager)
	pager.Finalize()
	pager.Finalize() // check idempotency of Finalize
	assert.Equal([]int{0, 1, 2, 3, 4}, arr)
	assert.True(morePages)
	assert.False(pager.CanConsume())
	assert.Panics(func() { pager.Consume(new(int)) })

	pager = consume.Page(3, 5, &arr, &morePages)
	feedInts(t, pager)
	pager.Finalize()
	assert.Equal([]int{15, 16, 17, 18, 19}, arr)
	assert.True(morePages)
	assert.False(pager.CanConsume())
	assert.Panics(func() { pager.Consume(new(int)) })

	pager = consume.Page(2, 5, &arr, &morePages)
	feedInts(t, consume.Slice(pager, 0, 15))
	pager.Finalize()
	assert.Equal([]int{10, 11, 12, 13, 14}, arr)
	assert.False(morePages)
	assert.False(pager.CanConsume())
	assert.Panics(func() { pager.Consume(new(int)) })

	pager = consume.Page(2, 5, &arr, &morePages)
	feedInts(t, consume.Slice(pager, 0, 11))
	pager.Finalize()
	assert.Equal([]int{10}, arr)
	assert.False(morePages)
	assert.False(pager.CanConsume())
	assert.Panics(func() { pager.Consume(new(int)) })

	pager = consume.Page(2, 5, &arr, &morePages)
	feedInts(t, consume.Slice(pager, 0, 10))
	pager.Finalize()
	assert.Equal([]int{}, arr)
	assert.False(morePages)
	assert.False(pager.CanConsume())
	assert.Panics(func() { pager.Consume(new(int)) })
}

func TestComposeUseIndividual(t *testing.T) {
	assert := assert.New(t)
	var strs []string
	var ints []int
	consumerOne := consume.MapFilter(
		consume.Slice(consume.AppendTo(&strs), 0, 1),
		func(src *int, dest *string) bool {
			*dest = strconv.Itoa(*src)
			return true
		})
	consumerThree := consume.Slice(consume.AppendTo(&ints), 0, 3)
	composite := consume.Compose(consumerOne, consumerThree, consume.Nil())
	assert.True(composite.CanConsume())
	i := 1
	composite.Consume(&i)
	assert.True(composite.CanConsume())
	i = 2
	composite.Consume(&i)
	assert.True(composite.CanConsume())
	i = 3

	// Use up individual consumer
	consumerThree.Consume(&i)

	// Now the composite consumer should return false
	assert.False(composite.CanConsume())

	assert.Equal([]string{"1"}, strs)
	assert.Equal([]int{1, 2, 3}, ints)
}

func TestConsumer(t *testing.T) {
	assert := assert.New(t)
	var zeroToFive []int
	var threeToSeven []int
	var sevensTo28 []int
	var timesTen []int
	var oneToThreePtr []*int
	onePtr := new(int)
	twoPtr := new(int)
	*onePtr = 1
	*twoPtr = 2
	consumer := consume.Compose(
		consume.Nil(),
		consume.MapFilter(
			consume.Slice(consume.AppendTo(&timesTen), 0, 100),
			func(src, dest *int) bool {
				*dest = (*src) * 10
				return true
			}),
		consume.Slice(consume.AppendTo(&zeroToFive), 0, 5),
		consume.Slice(consume.AppendTo(&threeToSeven), 3, 7),
		consume.Slice(consume.AppendPtrsTo(&oneToThreePtr), 1, 3),
		consume.MapFilter(
			consume.Slice(consume.AppendTo(&sevensTo28), 1, 4),
			func(ptr *int) bool { return (*ptr)%7 == 0 },
		))
	feedInts(t, consumer)
	assert.Equal([]int{0, 1, 2, 3, 4}, zeroToFive)
	assert.Equal([]int{3, 4, 5, 6}, threeToSeven)
	assert.Equal([]*int{onePtr, twoPtr}, oneToThreePtr)
	assert.Equal([]int{7, 14, 21}, sevensTo28)
	assert.Equal(10, timesTen[1])
	assert.Equal(20, timesTen[2])
	assert.Len(timesTen, 100)
}

func TestSlice(t *testing.T) {
	assert := assert.New(t)
	var zeroToFive []int
	feedInts(t, consume.Slice(consume.AppendTo(&zeroToFive), 0, 5))
	assert.Equal([]int{0, 1, 2, 3, 4}, zeroToFive)
}

func TestFilter(t *testing.T) {
	assert := assert.New(t)
	var sevensTo28 []int
	feedInts(t, consume.MapFilter(
		consume.Slice(consume.AppendTo(&sevensTo28), 1, 4),
		func(ptr *int) bool { return (*ptr)%7 == 0 }))
	assert.Equal([]int{7, 14, 21}, sevensTo28)
}

func TestMap(t *testing.T) {
	assert := assert.New(t)
	var zeroTo10By2 []string
	feedInts(t, consume.MapFilter(
		consume.Slice(consume.AppendTo(&zeroTo10By2), 0, 6),
		func(src *int, dest *string) bool {
			if (*src)%2 == 1 {
				return false
			}
			*dest = strconv.Itoa(*src)
			return true
		}))
	assert.Equal([]string{"0", "2", "4", "6", "8", "10"}, zeroTo10By2)
}

func TestMapFilter(t *testing.T) {
	assert := assert.New(t)
	var zeroTo150By30 []string
	feedInts(t, consume.MapFilter(
		consume.Slice(consume.AppendTo(&zeroTo150By30), 0, 6),
		consume.NewMapFilterer(),
		consume.NewMapFilterer(
			func(ptr *int) bool {
				return (*ptr)%2 == 0
			},
			consume.NewMapFilterer(func(ptr *int) bool {
				return (*ptr)%3 == 0
			}),
		),
		func(srcPtr *int, destPtr *string) bool {
			if (*srcPtr)%5 != 0 {
				return false
			}
			*destPtr = strconv.Itoa(*srcPtr)
			return true
		}))
	assert.Equal([]string{"0", "30", "60", "90", "120", "150"}, zeroTo150By30)
}

func TestNewMapFiltererPanics(t *testing.T) {
	assert := assert.New(t)
	assert.Panics(func() { consume.NewMapFilterer(3) })
	assert.Panics(func() {
		consume.NewMapFilterer(func() {})
	})
	assert.Panics(func() {
		consume.NewMapFilterer(func() int { return 4 })
	})
	assert.Panics(func() {
		consume.NewMapFilterer(func(x int) bool { return true })
	})
	assert.Panics(func() {
		consume.NewMapFilterer(func(ptr *int) {})
	})
	assert.Panics(func() {
		consume.NewMapFilterer(func(x, y, z *string) bool { return true })
	})
}

func ExampleMapFilter() {
	var evens []string
	consumer := consume.MapFilter(
		consume.AppendTo(&evens),
		func(ptr *int) bool {
			return (*ptr)%2 == 0
		},
		func(src *int, dest *string) bool {
			*dest = strconv.Itoa(*src)
			return true
		},
	)
	ints := []int{1, 2, 4}
	for _, i := range ints {
		if consumer.CanConsume() {
			consumer.Consume(&i)
		}
	}
	fmt.Println(evens)
	// Output: [2 4]
}

func feedInts(t *testing.T, consumer consume.Consumer) {
	assert := assert.New(t)
	idx := 0
	for consumer.CanConsume() {
		nidx := idx
		consumer.Consume(&nidx)
		idx++
	}
	assert.Panics(func() {
		consumer.Consume(&idx)
	})
}
