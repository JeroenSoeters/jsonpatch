package jsonpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONPatchCreate(t *testing.T) {
	cases := map[string]struct {
		a string
		b string
	}{
		"object": {
			`{"asdf":"qwerty"}`,
			`{"asdf":"zzz"}`,
		},
		"object with array": {
			`{"items":[{"asdf":"qwerty"}]}`,
			`{"items":[{"asdf":"bla"},{"asdf":"zzz"}]}`,
		},
		"array": {
			`[{"asdf":"qwerty"}]`,
			`[{"asdf":"bla"},{"asdf":"zzz"}]`,
		},
		"from empty array": {
			`[]`,
			`[{"asdf":"bla"},{"asdf":"zzz"}]`,
		},
		"to empty array": {
			`[{"asdf":"bla"},{"asdf":"zzz"}]`,
			`[]`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CreatePatch([]byte(tc.a), []byte(tc.b), false)
			assert.NoError(t, err)
		})
	}
}
