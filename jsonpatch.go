package jsonpatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

var errBadJSONDoc = fmt.Errorf("Invalid JSON Document")

type JsonPatchOperation struct {
	Operation string      `json:"op"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value,omitempty"`
}

func (j *JsonPatchOperation) Json() string {
	b, _ := json.Marshal(j)
	return string(b)
}

func (j *JsonPatchOperation) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{")
	b.WriteString(fmt.Sprintf(`"op":"%s"`, j.Operation))
	b.WriteString(fmt.Sprintf(`,"path":"%s"`, j.Path))
	// Consider omitting Value for non-nullable operations.
	if j.Value != nil || j.Operation == "replace" || j.Operation == "add" || j.Operation == "test" {
		v, err := json.Marshal(j.Value)
		if err != nil {
			return nil, err
		}
		b.WriteString(`,"value":`)
		b.Write(v)
	}
	b.WriteString("}")
	return b.Bytes(), nil
}

type ByPath []JsonPatchOperation

func (a ByPath) Len() int           { return len(a) }
func (a ByPath) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPath) Less(i, j int) bool { return a[i].Path < a[j].Path }

func NewPatch(operation, path string, value interface{}) JsonPatchOperation {
	return JsonPatchOperation{Operation: operation, Path: path, Value: value}
}

// CreatePatch creates a patch as specified in http://jsonpatch.com/
//
// 'a' is original, 'b' is the modified document. Both are to be given as json encoded content.
// The function will return an array of JsonPatchOperations
// If ignoreArrayOrder is true, arrays with the same elements but in different order will be considered equal
//
// An error will be returned if any of the two documents are invalid.
func CreatePatch(a, b []byte, ignoreArrayOrder bool) ([]JsonPatchOperation, error) {
	var aI interface{}
	var bI interface{}

	err := json.Unmarshal(a, &aI)
	if err != nil {
		return nil, errBadJSONDoc
	}
	err = json.Unmarshal(b, &bI)
	if err != nil {
		return nil, errBadJSONDoc
	}

	return handleValues(aI, bI, "", []JsonPatchOperation{}, ignoreArrayOrder)
}

// Returns true if the values matches (must be json types)
// The types of the values must match, otherwise it will always return false
// If two map[string]interface{} are given, all elements must match.
// If ignoreArrayOrder is true and both values are arrays, they are compared as sets
func matchesValue(av, bv interface{}, ignoreArrayOrder bool) bool {
	if reflect.TypeOf(av) != reflect.TypeOf(bv) {
		return false
	}
	switch at := av.(type) {
	case string:
		bt := bv.(string)
		if bt == at {
			return true
		}
	case float64:
		bt := bv.(float64)
		if bt == at {
			return true
		}
	case bool:
		bt := bv.(bool)
		if bt == at {
			return true
		}
	case map[string]interface{}:
		bt := bv.(map[string]interface{})
		for key := range at {
			if !matchesValue(at[key], bt[key], ignoreArrayOrder) {
				return false
			}
		}
		for key := range bt {
			if !matchesValue(at[key], bt[key], ignoreArrayOrder) {
				return false
			}
		}
		return true
	case []interface{}:
		bt := bv.([]interface{})
		if len(bt) != len(at) {
			return false
		}

		if ignoreArrayOrder {
			// Check if arrays have the same elements, regardless of order
			// Create a map of element counts for each array
			atCount := make(map[string]int)
			btCount := make(map[string]int)

			// Count elements in first array
			for _, v := range at {
				// Convert element to JSON string for comparison
				jsonBytes, err := json.Marshal(v)
				if err != nil {
					return false
				}
				jsonStr := string(jsonBytes)
				atCount[jsonStr]++
			}

			// Count elements in second array
			for _, v := range bt {
				jsonBytes, err := json.Marshal(v)
				if err != nil {
					return false
				}
				jsonStr := string(jsonBytes)
				btCount[jsonStr]++
			}

			// Compare counts
			if len(atCount) != len(btCount) {
				return false
			}

			for k, v := range atCount {
				if btCount[k] != v {
					return false
				}
			}

			return true
		} else {
			// Order matters, check each element in order
			for key := range at {
				if !matchesValue(at[key], bt[key], ignoreArrayOrder) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// From http://tools.ietf.org/html/rfc6901#section-4 :
//
// Evaluation of each reference token begins by decoding any escaped
// character sequence.  This is performed by first transforming any
// occurrence of the sequence '~1' to '/', and then transforming any
// occurrence of the sequence '~0' to '~'.
//   TODO decode support:
//   var rfc6901Decoder = strings.NewReplacer("~1", "/", "~0", "~")

var rfc6901Encoder = strings.NewReplacer("~", "~0", "/", "~1")

func makePath(path string, newPart interface{}) string {
	key := rfc6901Encoder.Replace(fmt.Sprintf("%v", newPart))
	if path == "" {
		return "/" + key
	}
	if strings.HasSuffix(path, "/") {
		return path + key
	}
	return path + "/" + key
}

// diff returns the (recursive) difference between a and b as an array of JsonPatchOperations.
func diff(a, b map[string]interface{}, path string, patch []JsonPatchOperation, ignoreArrayOrder bool) ([]JsonPatchOperation, error) {
	for key, bv := range b {
		p := makePath(path, key)
		av, ok := a[key]
		// value was added
		if !ok {
			patch = append(patch, NewPatch("add", p, bv))
			continue
		}
		// If types have changed, replace completely
		if reflect.TypeOf(av) != reflect.TypeOf(bv) {
			patch = append(patch, NewPatch("replace", p, bv))
			continue
		}
		// Types are the same, compare values
		var err error
		patch, err = handleValues(av, bv, p, patch, ignoreArrayOrder)
		if err != nil {
			return nil, err
		}
	}
	// Now add all deleted values as nil
	for key := range a {
		_, found := b[key]
		if !found {
			p := makePath(path, key)

			patch = append(patch, NewPatch("remove", p, nil))
		}
	}
	return patch, nil
}

func handleValues(av, bv interface{}, p string, patch []JsonPatchOperation, ignoreArrayOrder bool) ([]JsonPatchOperation, error) {
	var err error
	switch at := av.(type) {
	case map[string]interface{}:
		bt := bv.(map[string]interface{})
		patch, err = diff(at, bt, p, patch, ignoreArrayOrder)
		if err != nil {
			return nil, err
		}
	case string, float64, bool:
		if !matchesValue(av, bv, ignoreArrayOrder) {
			patch = append(patch, NewPatch("replace", p, bv))
		}
	case []interface{}:
		bt, ok := bv.([]interface{})
		if !ok {
			// array replaced by non-array
			patch = append(patch, NewPatch("replace", p, bv))
		} else if len(at) != len(bt) {
			// arrays are not the same length
			patch = append(patch, compareArray(at, bt, p, ignoreArrayOrder)...)
		} else if ignoreArrayOrder && matchesValue(at, bt, true) {
			// Arrays have the same elements, just in different order, and we're ignoring order
			// No patch needed!
		} else {
			for i := range bt {
				patch, err = handleValues(at[i], bt[i], makePath(p, i), patch, ignoreArrayOrder)
				if err != nil {
					return nil, err
				}
			}
		}
	case nil:
		switch bv.(type) {
		case nil:
			// Both nil, fine.
		default:
			patch = append(patch, NewPatch("add", p, bv))
		}
	default:
		panic(fmt.Sprintf("Unknown type:%T ", av))
	}
	return patch, nil
}

// compareArray generates remove and add operations for `av` and `bv`.
func compareArray(av, bv []interface{}, p string, ignoreArrayOrder bool) []JsonPatchOperation {
	retval := []JsonPatchOperation{}

	// If arrays have same elements in different order and we're ignoring order, return empty patch
	if ignoreArrayOrder && len(av) == len(bv) && matchesValue(av, bv, true) {
		return retval
	}

	// Find elements that need to be removed
	processArray(av, bv, func(i int, value interface{}) {
		retval = append(retval, NewPatch("remove", makePath(p, i), nil))
	}, ignoreArrayOrder)

	reversed := make([]JsonPatchOperation, len(retval))
	for i := 0; i < len(retval); i++ {
		reversed[len(retval)-1-i] = retval[i]
	}
	retval = reversed

	// Find elements that need to be added.
	// NOTE we pass in `bv` then `av` so that processArray can find the missing elements.
	processArray(bv, av, func(i int, value interface{}) {
		retval = append(retval, NewPatch("add", makePath(p, i), value))
	}, ignoreArrayOrder)

	return retval
}

// processArray processes `av` and `bv` calling `applyOp` whenever a value is absent.
// It keeps track of which indexes have already had `applyOp` called for and automatically skips them so you can process duplicate objects correctly.
func processArray(av, bv []interface{}, applyOp func(i int, value interface{}), ignoreArrayOrder bool) {
	foundIndexes := make(map[int]struct{}, len(av))
	reverseFoundIndexes := make(map[int]struct{}, len(av))

	if ignoreArrayOrder {
		// Create a map of elements and their counts in bv
		bvCounts := make(map[string]int)
		bvSeen := make(map[string]int) // Track how many we've seen during processing

		for _, v := range bv {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				continue // Skip if we can't marshal
			}
			jsonStr := string(jsonBytes)
			bvCounts[jsonStr]++
		}

		// Check each element in av
		for i, v := range av {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				applyOp(i, v) // If we can't marshal, treat it as not found
				continue
			}

			jsonStr := string(jsonBytes)
			// If element exists in bv and we haven't seen all of them yet
			if bvCounts[jsonStr] > bvSeen[jsonStr] {
				foundIndexes[i] = struct{}{}
				bvSeen[jsonStr]++
			}
		}

		// Apply op for all elements in av that weren't found
		for i, v := range av {
			if _, ok := foundIndexes[i]; !ok {
				applyOp(i, v)
			}
		}
	} else {
		// Original implementation for when order matters
		for i, v := range av {
			for i2, v2 := range bv {
				if _, ok := reverseFoundIndexes[i2]; ok {
					// We already found this index.
					continue
				}
				if reflect.DeepEqual(v, v2) {
					// Mark this index as found since it matches exactly.
					foundIndexes[i] = struct{}{}
					reverseFoundIndexes[i2] = struct{}{}
					break
				}
			}
			if _, ok := foundIndexes[i]; !ok {
				applyOp(i, v)
			}
		}
	}
}
