package jsonpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var simpleA = `{"a":100, "b":200, "c":"hello"}`
var simpleB = `{"a":100, "b":200, "c":"goodbye"}`
var simpleC = `{"a":100, "b":100, "c":"hello"}`
var simpleD = `{"a":100, "b":200, "c":"hello", "d":"foo"}`
var simpleE = `{"a":100, "b":200}`
var simplef = `{"a":100, "b":100, "d":"foo"}`
var simpleG = `{"a":100, "b":null, "d":"foo"}`
var empty = `{}`

func TestOneNullReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(simplef), []byte(simpleG), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/b", "they should be equal")
	assert.Equal(t, change.Value, nil, "they should be equal")
}

func TestSame(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleA), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 0, "they should be equal")
}

func TestOneStringReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleB), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/c", "they should be equal")
	assert.Equal(t, change.Value, "goodbye", "they should be equal")
}

func TestOneIntReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleC), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/b", "they should be equal")
	var expected float64 = 100
	assert.Equal(t, change.Value, expected, "they should be equal")
}

func TestOneAdd(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleD), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "add", "they should be equal")
	assert.Equal(t, change.Path, "/d", "they should be equal")
	assert.Equal(t, change.Value, "foo", "they should be equal")
}

// We never remove properties from objects
func TestOneRemove(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleE), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 0, "they should be equal")
	// change := patch[0]
	// assert.Equal(t, change.Operation, "remove", "they should be equal")
	// assert.Equal(t, change.Path, "/c", "they should be equal")
	// assert.Equal(t, change.Value, nil, "they should be equal")
}

// We never remove properties from objects
func TestVsEmpty(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(empty), Collections{}, PatchStrategyExactMatch)
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 0, "they should be equal")
	// sort.Sort(ByPath(patch))
	// change := patch[0]
	// assert.Equal(t, change.Operation, "remove", "they should be equal")
	// assert.Equal(t, change.Path, "/a", "they should be equal")
	//
	// change = patch[1]
	// assert.Equal(t, change.Operation, "remove", "they should be equal")
	// assert.Equal(t, change.Path, "/b", "they should be equal")
	//
	// change = patch[2]
	// assert.Equal(t, change.Operation, "remove", "they should be equal")
	// assert.Equal(t, change.Path, "/c", "they should be equal")
}

func BenchmarkBigArrays(b *testing.B) {
	var a1, a2 []any
	a1 = make([]any, 100)
	a2 = make([]any, 101)

	for i := range 100 {
		a1[i] = i
		a2[i+1] = i
	}
	for i := 0; i < b.N; i++ {
		compareArray(a1, a2, "/", PatchStrategyExactMatch, Collections{})
	}
}

func BenchmarkBigArrays2(b *testing.B) {
	var a1, a2 []any
	a1 = make([]any, 100)
	a2 = make([]any, 101)

	for i := range 100 {
		a1[i] = i
		a2[i] = i
	}
	for i := 0; i < b.N; i++ {
		compareArray(a1, a2, "/", PatchStrategyExactMatch, Collections{})
	}
}
