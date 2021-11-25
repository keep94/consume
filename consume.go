// Package consume provides useful ways to consume values.
package consume

import (
	"reflect"
)

const (
	kCantConsume         = "Can't consume"
	kParamMustReturnBool = "Parameter must return bool"
)

// Consumer consumes values.
type Consumer interface {

	// CanConsume returns true if this instance can consume a value.
	// Once CanConsume returns false, it should always return false.
	CanConsume() bool

	// Consume consumes the value that ptr points to. Consume panics if
	// CanConsume() returns false.
	Consume(ptr interface{})
}

// ConsumeFinalizer adds a Finalize method to Consumer.
type ConsumeFinalizer interface {
	Consumer

	// Caller calls Finalize after it is done passing values to this consumer.
	// Once caller calls Finalize(), CanConsume() returns false and Consume()
	// panics. Calls to Finalize are idempotent.
	Finalize()
}

// The ConsumerFunc type is an adapter to allow the use of an ordinary function
// as a Consumer. ConsumerFunc can always consume.
type ConsumerFunc func(ptr interface{})

// Consume invokes c, this function.
func (c ConsumerFunc) Consume(ptr interface{}) {
	c(ptr)
}

// CanConsume always returns true.
func (c ConsumerFunc) CanConsume() bool {
	return true
}

// MustCanConsume panics if c cannot consume
func MustCanConsume(c Consumer) {
	if !c.CanConsume() {
		panic(kCantConsume)
	}
}

// Nil returns a consumer that consumes nothing. Calling CanConsume() on
// returned consumer returns false, and calling Consume() on returned
// consumer panics.
func Nil() Consumer {
	return nilConsumer{}
}

// AppendToSaveMemory works like AppendTo but saves on allocs. AppendTo does
// O(N) allocs where N is the number of items being appended.
// AppendToSaveMemory does only O(log N) worst case. It accomplishes this
// by not modifying the dimensions of the slice with every append. Because
// of this, caller must call Finalize() on the returned consumer when
// appending is finished so that it can adjust the dimensions of the slice
// one last time to fit the items appended. Not calling Finalize() results in
// a slice that has extra elements at the end.
func AppendToSaveMemory(aValueSlicePointer interface{}) ConsumeFinalizer {
	aSliceValue := sliceValueFromP(aValueSlicePointer, false)
	length := aSliceValue.Len()
	if aSliceValue.Cap() < 4 {
		truncateTo(aSliceValue, 4)
	} else {
		truncateTo(aSliceValue, aSliceValue.Cap())
	}
	return &appendSaveMemoryConsumer{
		buffer: aSliceValue, length: length}
}

// AppendTo returns a Consumer that appends consumed values to the slice
// pointed to by aValueSlicePointer. aValueSlicePointer is a pointer to a
// slice of values supporting assignment. The CanConsume method of returned
// consumer always returns true.
func AppendTo(aValueSlicePointer interface{}) Consumer {
	aSliceValue := sliceValueFromP(aValueSlicePointer, false)
	return &appendConsumer{buffer: aSliceValue}
}

// AppendPtrsTo returns a Consumer that appends consumed values to the slice
// pointed to by aPointerSlicePointer. Each time the returned Consumer
// consumes a value, it allocates a new value on the heap, copies the
// consumed value to that allocated value, and finally appends the pointer
// to the newly allocated value to the slice pointed to by
// aPointerSlicePointer. aPointerSlicePointer is a pointer to a slice of
// pointers to values supporting assignment. The CanConsume method of
// returned consumer always returns true.
func AppendPtrsTo(aPointerSlicePointer interface{}) Consumer {
	aSliceValue := sliceValueFromP(aPointerSlicePointer, true)
	aSliceType := aSliceValue.Type()
	allocType := aSliceType.Elem().Elem()
	return &appendConsumer{buffer: aSliceValue, allocType: allocType}
}

// Compose returns the consumers passed to it as a single Consumer. When
// returned consumer consumes a value, each consumer passed in that is able to
// consume a value consumes that value. CanConsume() of returned consumer
// returns false when the CanConsume() method of each consumer passed in
// returns false.
func Compose(consumers ...Consumer) Consumer {
	clen := len(consumers)
	switch clen {
	case 0:
		return nilConsumer{}
	case 1:
		return consumers[0]
	default:
		consumerList := make([]Consumer, clen)
		copy(consumerList, consumers)
		return &multiConsumer{consumers: consumerList}
	}
}

// Slice returns a Consumer that passes the start th value consumed
// inclusive to the end th value consumed exclusive onto consumer where start
// and end are zero based. The returned consumer ignores the first
// start values it consumes. After that it passes the values it consumes
// onto consumer until it has consumed end values. The CanConsume() method
// of returned consumer returns false if the CanConsume() method of the
// underlying consumer returns false or if the returned consumer has consumed
// end values. Note that if end <= start, the underlying consumer will never
// get any values. A negative start or end is treated as 0.
func Slice(consumer Consumer, start, end int) Consumer {
	return &sliceConsumer{consumer: consumer, start: start, end: end}
}

// MapFilterer represents zero or more functions like the ones passed
// to MapFilter chained together.
type MapFilterer interface {

	// MapFilter applies the chained filter and map functions to what ptr
	// points to while leaving it unchanged. MapFilter returns nil if ptr
	// should be filtered out; returns ptr itself; or returns a pointer to
	// a mapped value. If MapFilter returns a pointer to a mapped value,
	// it gets overwritten with each call to MapFilter because such values
	// are stored within this MapFilterer to avoid memory allocations.
	MapFilter(ptr interface{}) interface{}

	addClones(result *[]MapFilterer)
	size() int
}

// Mapper maps a value to a new value.
type Mapper interface {

	// Map takes a pointer to a value and returns a pointer to the
	// mapped value. Map returns the same pointer every time, but the
	// value that pointer points to changes.
	Map(ptr interface{}) interface{}

	// Clone clones this Mapper. The Map method of the cloned Mapper
	// returns a different pointer from the original.
	Clone() Mapper
}

// Filterer filters a value
type Filterer interface {

	// Filter returns true if the value ptr points to should be included
	// or false otherwise.
	Filter(ptr interface{}) bool
}

// NewMapFilterer creates a MapFilterer from multiple functions like the ones
// passed to MapFilter chained together. The returned MapFilterer can be
// passed as a parameter to MapFilter or to NewMapFilterer. The returned
// MapFilterer contains copies of any MapFilterers passed in. Therefore,
// the returned MapFilterer and any passed in MapFilterers can safely be used
// at the same time.
func NewMapFilterer(funcs ...interface{}) MapFilterer {
	resultSize := 0
	for _, f := range funcs {
		resultSize += mfSize(f)
	}
	result := make([]MapFilterer, 0, resultSize)
	for _, f := range funcs {
		mfAddClones(f, &result)
	}
	switch len(result) {
	case 0:
		return nilMapFilterer{}
	case 1:
		return result[0]
	default:
		return sliceMapFilterer(result)
	}
}

// MapFilter returns a Consumer that passes only filtered and mapped
// values onto the consumer parameter. The returned Consumer applies
// each function in funcs to the value passed to its Consume method.
// The resulting value is then passed to the Consume method of the
// consumer parameter. Each function in func returns a bool and takes
// one or two pointer arguments to values. If a function returns false,
// it means that the value passed to it should not be consumed. The one
// argument functions never change the value passed to it as they are
// simple filters. The 2 argument functions are mappers. They leave their
// first argument unchanged, but use it to set their second argument.
// This second argument is what gets passed to the next function in funcs
// or to consumer if it is the last function in funcs.
//
// MapFilter can also take Mapper or Filterer interfaces instead of raw
// functions. These interfaces exist because calling a raw function via
// reflection costs two allocations for each value consumed.
//
// The NewMapFilterer function can return a MapFilterer which represents zero
// or more of these functions chained together. MapFilterer instances can be
// passed as parameters to MapFilter just like the functions mentioned above.
// The MapFilter function takes a copy of any passed in MapFilterer instance,
// so any passed MapFilterer instance can safely be used at the same time as
// the returned Consumer.
func MapFilter(consumer Consumer, funcs ...interface{}) Consumer {
	mapFilters := NewMapFilterer(funcs...)
	if mapFilters.size() == 0 {
		return consumer
	}
	return &mapFilterConsumer{
		Consumer:   consumer,
		mapFilters: mapFilters,
	}
}

// TakeWhile works like MapFilter except that returned Consumer only
// accepts values until one is filtered out. Once returned Consumer filters
// out a value, its CanConsume() method always returns false.
func TakeWhile(consumer Consumer, funcs ...interface{}) Consumer {
	mapFilters := NewMapFilterer(funcs...)
	if mapFilters.size() == 0 {
		return consumer
	}
	return &takeWhileConsumer{
		consumer:   consumer,
		mapFilters: mapFilters,
	}
}

// Page returns a consumer that does pagination. The items in page fetched
// get stored in the slice pointed to by aValueSlicePointer. Note that
// aValueSlicePointer is a pointer to a slice of values that support
// assignment. If there are more pages after page fetched, Page sets morePages
// to true; otherwise, it sets morePages to false. Note that the values stored
// at aValueSlicePointer and morePages are undefined until caller calls
// Finalize() on returned ConsumeFinalizer. Page panics if zeroBasedPageNo
// is negative, if itemsPerPage <= 0, or if aValueSlicePointer is not a
// pointer to a slice.
func Page(
	zeroBasedPageNo int,
	itemsPerPage int,
	aValueSlicePointer interface{},
	morePages *bool) ConsumeFinalizer {
	if zeroBasedPageNo < 0 {
		panic("zeroBasedPageNo must be non-negative")
	}
	if itemsPerPage <= 0 {
		panic("itemsPerPage must be positive")
	}
	aSliceValue := sliceValueFromP(aValueSlicePointer, false)
	ensureEmptyWithCapacity(aSliceValue, itemsPerPage+1)
	cf := AppendToSaveMemory(aValueSlicePointer)
	consumer := Slice(
		cf,
		zeroBasedPageNo*itemsPerPage,
		(zeroBasedPageNo+1)*itemsPerPage+1)
	return &pageConsumer{
		Consumer:     consumer,
		cf:           cf,
		itemsPerPage: itemsPerPage,
		aSliceValue:  aSliceValue,
		morePages:    morePages}
}

type pageConsumer struct {
	Consumer
	cf           ConsumeFinalizer
	itemsPerPage int
	aSliceValue  reflect.Value
	morePages    *bool
	finalized    bool
}

func (p *pageConsumer) Finalize() {
	if p.finalized {
		return
	}
	p.finalized = true
	p.cf.Finalize()
	p.Consumer = nilConsumer{}
	if p.aSliceValue.Len() == p.itemsPerPage+1 {
		*p.morePages = true
		truncateTo(p.aSliceValue, p.itemsPerPage)
	} else {
		*p.morePages = false
	}
}

func ensureEmptyWithCapacity(aSliceValue reflect.Value, capacity int) {
	if aSliceValue.Cap() < capacity {
		typ := aSliceValue.Type()
		aSliceValue.Set(reflect.MakeSlice(typ, 0, capacity))
	} else {
		truncateTo(aSliceValue, 0)
	}
}

func truncateTo(aSliceValue reflect.Value, newLength int) {
	if newLength <= aSliceValue.Cap() {
		aSliceValue.Set(aSliceValue.Slice(0, newLength))
		return
	}
	newSlice := reflect.MakeSlice(aSliceValue.Type(), newLength, newLength)
	reflect.Copy(newSlice, aSliceValue)
	aSliceValue.Set(newSlice)
}

type sliceConsumer struct {
	consumer Consumer
	start    int
	end      int
	idx      int
}

func (s *sliceConsumer) CanConsume() bool {
	return s.consumer.CanConsume() && s.idx < s.end
}

func (s *sliceConsumer) Consume(ptr interface{}) {
	MustCanConsume(s)
	if s.idx >= s.start {
		s.consumer.Consume(ptr)
	}
	s.idx++
}

type multiConsumer struct {
	consumers []Consumer
}

func (m *multiConsumer) CanConsume() bool {
	m.filterFinished()
	return len(m.consumers) > 0
}

func (m *multiConsumer) Consume(ptr interface{}) {
	MustCanConsume(m)
	for _, consumer := range m.consumers {
		consumer.Consume(ptr)
	}
}

func (m *multiConsumer) filterFinished() {
	idx := 0
	for i := range m.consumers {
		if m.consumers[i].CanConsume() {
			m.consumers[idx] = m.consumers[i]
			idx++
		}
	}
	for i := idx; i < len(m.consumers); i++ {
		m.consumers[i] = nil
	}
	m.consumers = m.consumers[0:idx]
}

type appendConsumer struct {
	buffer    reflect.Value
	allocType reflect.Type
}

func (a *appendConsumer) CanConsume() bool {
	return true
}

func (a *appendConsumer) Consume(ptr interface{}) {
	valueToConsume := reflect.ValueOf(ptr).Elem()
	if a.allocType == nil {
		a.buffer.Set(reflect.Append(a.buffer, valueToConsume))
	} else {
		newPtr := reflect.New(a.allocType)
		newPtr.Elem().Set(valueToConsume)
		a.buffer.Set(reflect.Append(a.buffer, newPtr))
	}
}

type appendSaveMemoryConsumer struct {
	buffer    reflect.Value
	length    int
	finalized bool
}

func (a *appendSaveMemoryConsumer) CanConsume() bool {
	return !a.finalized
}

func (a *appendSaveMemoryConsumer) Consume(ptr interface{}) {
	if a.finalized {
		panic(kCantConsume)
	}
	if a.length == a.buffer.Len() {
		truncateTo(a.buffer, 2*a.length)
	}
	a.buffer.Index(a.length).Set(reflect.ValueOf(ptr).Elem())
	a.length++
}

func (a *appendSaveMemoryConsumer) Finalize() {
	if a.finalized {
		return
	}
	a.finalized = true
	truncateTo(a.buffer, a.length)
}

func sliceValueFromP(
	aSlicePointer interface{}, sliceOfPtrs bool) reflect.Value {
	resultPtr := reflect.ValueOf(aSlicePointer)
	if resultPtr.Type().Kind() != reflect.Ptr {
		panic("A pointer to a slice is expected.")
	}
	return checkSliceValue(resultPtr.Elem(), sliceOfPtrs)
}

func checkSliceValue(
	result reflect.Value, sliceOfPtrs bool) reflect.Value {
	if result.Kind() != reflect.Slice {
		panic("a slice is expected.")
	}
	if sliceOfPtrs && result.Type().Elem().Kind() != reflect.Ptr {
		panic("a slice of pointers is expected.")
	}
	return result
}

type nilConsumer struct {
}

func (n nilConsumer) CanConsume() bool {
	return false
}

func (n nilConsumer) Consume(ptr interface{}) {
	panic(kCantConsume)
}

func mfSize(f interface{}) int {
	if mf, ok := f.(MapFilterer); ok {
		return mf.size()
	}
	return 1
}

func mfAddClones(f interface{}, result *[]MapFilterer) {
	if mf, ok := f.(MapFilterer); ok {
		mf.addClones(result)
		return
	}
	if aMapper, ok := f.(Mapper); ok {
		*result = append(
			*result, &mapperInterfaceWrapper{value: aMapper.Clone()})
		return
	}
	if aFilterer, ok := f.(Filterer); ok {
		*result = append(
			*result, &filtererInterfaceWrapper{value: aFilterer})
		return
	}
	*result = append(*result, newMapFilterer(f))
}

func newMapFilterer(f interface{}) MapFilterer {
	fvalue := reflect.ValueOf(f)
	ftype := reflect.TypeOf(f)
	validateFuncType(ftype)
	numIn := ftype.NumIn()
	if numIn == 1 {
		return &filterer{
			value: fvalue,
		}
	} else if numIn == 2 {
		resultType := ftype.In(1).Elem()
		resultPtr := reflect.New(resultType)
		return &mapper{
			value:      fvalue,
			resultType: resultType,
			resultPtr:  resultPtr,
			iresultPtr: resultPtr.Interface(),
		}
	} else {
		panic("Function parameter must take 1 or 2 parameters")
	}
}

func validateFuncType(ftype reflect.Type) {
	if ftype.Kind() != reflect.Func {
		panic("Parameter must be a function")
	}
	if ftype.NumOut() != 1 {
		panic(kParamMustReturnBool)
	}
	if ftype.Out(0) != reflect.TypeOf(true) {
		panic(kParamMustReturnBool)
	}
	numIn := ftype.NumIn()
	for i := 0; i < numIn; i++ {
		if ftype.In(i).Kind() != reflect.Ptr {
			panic("Function parameter must accept pointer arguments")
		}
	}
}

type filtererInterfaceWrapper struct {
	value Filterer
}

func (f *filtererInterfaceWrapper) MapFilter(ptr interface{}) interface{} {
	if f.value.Filter(ptr) {
		return ptr
	}
	return nil
}

func (f *filtererInterfaceWrapper) size() int { return 1 }

func (f *filtererInterfaceWrapper) addClones(result *[]MapFilterer) {
	*result = append(*result, f)
}

type mapperInterfaceWrapper struct {
	value Mapper
}

func (m *mapperInterfaceWrapper) MapFilter(ptr interface{}) interface{} {
	return m.value.Map(ptr)
}

func (m *mapperInterfaceWrapper) size() int { return 1 }

func (m *mapperInterfaceWrapper) addClones(result *[]MapFilterer) {
	*result = append(*result, m.clone())
}

func (m *mapperInterfaceWrapper) clone() *mapperInterfaceWrapper {
	return &mapperInterfaceWrapper{value: m.value.Clone()}
}

type filterer struct {
	value reflect.Value
}

func (f *filterer) MapFilter(ptr interface{}) interface{} {
	params := [...]reflect.Value{reflect.ValueOf(ptr)}
	if f.value.Call(params[:])[0].Bool() {
		return ptr
	}
	return nil
}

func (f *filterer) size() int { return 1 }

func (f *filterer) addClones(result *[]MapFilterer) {
	*result = append(*result, f)
}

type mapper struct {
	value      reflect.Value
	resultType reflect.Type
	resultPtr  reflect.Value
	iresultPtr interface{}
}

func (m *mapper) MapFilter(ptr interface{}) interface{} {
	params := [...]reflect.Value{reflect.ValueOf(ptr), m.resultPtr}
	if m.value.Call(params[:])[0].Bool() {
		return m.iresultPtr
	}
	return nil
}

func (m *mapper) size() int { return 1 }

func (m *mapper) addClones(result *[]MapFilterer) {
	*result = append(*result, m.clone())
}

func (m *mapper) clone() *mapper {
	result := *m
	result.resultPtr = reflect.New(result.resultType)
	result.iresultPtr = result.resultPtr.Interface()
	return &result
}

type sliceMapFilterer []MapFilterer

func (s sliceMapFilterer) MapFilter(ptr interface{}) interface{} {
	for _, mf := range s {
		ptr = mf.MapFilter(ptr)
		if ptr == nil {
			return nil
		}
	}
	return ptr
}

func (s sliceMapFilterer) size() int { return len(s) }

func (s sliceMapFilterer) addClones(result *[]MapFilterer) {
	for _, mf := range s {
		mf.addClones(result)
	}
}

type nilMapFilterer struct{}

func (n nilMapFilterer) MapFilter(ptr interface{}) interface{} {
	return ptr
}

func (n nilMapFilterer) size() int { return 0 }

func (n nilMapFilterer) addClones(result *[]MapFilterer) {}

type mapFilterConsumer struct {
	Consumer
	mapFilters MapFilterer
}

func (m *mapFilterConsumer) Consume(ptr interface{}) {
	MustCanConsume(m)
	ptr = m.mapFilters.MapFilter(ptr)
	if ptr == nil {
		return
	}
	m.Consumer.Consume(ptr)
}

type takeWhileConsumer struct {
	consumer   Consumer
	mapFilters MapFilterer
	done       bool
}

func (t *takeWhileConsumer) CanConsume() bool {
	return t.consumer.CanConsume() && !t.done
}

func (t *takeWhileConsumer) Consume(ptr interface{}) {
	MustCanConsume(t)
	ptr = t.mapFilters.MapFilter(ptr)
	if ptr == nil {
		t.done = true
		return
	}
	t.consumer.Consume(ptr)
}
