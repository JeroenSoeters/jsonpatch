package jsonpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalNullableValue(t *testing.T) {
	//	p1 := JsonPatchOperation{
	//		Operation: "replace",
	//		Path:      "/a1",
	//		Value:     nil,
	//	}
	//	p1json := p1.Json()
	//	assert.JSONEq(t, `{"op":"replace", "path":"/a1","value":null}`, p1json)

	p2 := JsonPatchOperation{
		Operation: "replace",
		Path:      "/a2",
		Value:     "v2",
	}
	assert.JSONEq(t, `{"op":"replace", "path":"/a2", "value":"v2"}`, p2.Json())
}

func TestMarshalNonNullableValue(t *testing.T) {
	p1 := JsonPatchOperation{
		Operation: "remove",
		Path:      "/a1",
	}
	assert.JSONEq(t, `{"op":"remove", "path":"/a1"}`, p1.Json())

}
