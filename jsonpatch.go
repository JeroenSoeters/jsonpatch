package jsonpatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var errBadJsonDoc = fmt.Errorf("Invalid Json Document")

type Path string
type Key string
type EntitySets map[Path]Key

type Collections struct {
	EntitySets    EntitySets
	Arrays        []Path
	IgnoredFields []Path
}

func (c *Collections) isArray(path string) bool {
	jsonPath := toJsonPath(path)
	return slices.Contains(c.Arrays, Path(jsonPath))
}

func (c *Collections) isEntitySet(path string) bool {
	jsonPath := toJsonPath(path)
	_, ok := c.EntitySets[Path(jsonPath)]
	return ok
}

func (s EntitySets) Add(path Path, key Key) {
	if s == nil {
		s = make(EntitySets)
	}
	s[path] = key
}

func (s EntitySets) Get(path Path) (Key, bool) {
	if s == nil {
		return "", false
	}
	key, ok := s[path]
	return key, ok
}

func toJsonPath(path string) string {
	if path == "" || path == "/" {
		return "$"
	}

	parts := strings.Split(path, "/")
	var jsonPathParts []string

	for _, part := range parts {
		if part == "" {
			continue
		}

		_, err := strconv.Atoi(part)
		if err == nil {
			jsonPathParts = append(jsonPathParts, "[*]")
		} else {
			jsonPathParts = append(jsonPathParts, "."+part)
		}
	}

	return "$" + strings.Join(jsonPathParts, "")
}

type PatchStrategy string

const (
	PatchStrategyExactMatch   PatchStrategy = "exact-match"
	PatchStrategyEnsureExists PatchStrategy = "ensure-exists"
	PatchStrategyEnsureAbsent PatchStrategy = "ensure-absent"
)

type JsonPatchOperation struct {
	Operation string `json:"op"`
	Path      string `json:"path"`
	Value     any    `json:"value,omitempty"`
}

func (j *JsonPatchOperation) Json() string {
	b, _ := json.Marshal(j)
	return string(b)
}

func (j *JsonPatchOperation) MarshalJson() ([]byte, error) {
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

func NewPatch(operation, path string, value any) JsonPatchOperation {
	return JsonPatchOperation{Operation: operation, Path: path, Value: value}
}

// CreatePatch creates a patch as specified in http://jsonpatch.com/
//
// 'a' is original, 'b' is the modified document. Both are to be given as json encoded content.
// The function will return an array of JsonPatchOperations
// If ignoreArrayOrder is true, arrays with the same elements but in different order will be considered equal
//
// An e rror will be returned if any of the two documents are invalid.
func CreatePatch(a, b []byte, collections Collections, strategy PatchStrategy) ([]JsonPatchOperation, error) {
	var aUnmarshalled any
	var bUnmarshalled any

	err := json.Unmarshal(a, &aUnmarshalled)
	if err != nil {
		return nil, errBadJsonDoc
	}
	err = json.Unmarshal(b, &bUnmarshalled)
	if err != nil {
		return nil, errBadJsonDoc
	}
	aWithoutIgnoredFields, err := removeIgnoredFields(aUnmarshalled, collections.IgnoredFields)
	if err != nil {
		return nil, fmt.Errorf("error removing ignored fields from original document: %w", err)
	}
	bWithoutIgnoredFields, err := removeIgnoredFields(bUnmarshalled, collections.IgnoredFields)
	if err != nil {
		return nil, fmt.Errorf("error removing ignored fields from modified document: %w", err)
	}

	return handleValues(aWithoutIgnoredFields, bWithoutIgnoredFields, "", []JsonPatchOperation{}, strategy, collections)
}

// Returns true if the values matches (must be json types)
// The types of the values must match, otherwise it will always return false
// If two map[string]any are given, all elements must match.
// If ignoreArrayOrder is true and both values are arrays, they are compared as sets
func matchesValue(av, bv any, ignoreArrayOrder bool) bool {
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
	case map[string]any:
		bt := bv.(map[string]any)
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
	case []any:
		bt := bv.([]any)
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
				// Convert element to Json string for comparison
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
		}
		// Order matters, check each element in order
		for key := range at {
			if !matchesValue(at[key], bt[key], ignoreArrayOrder) {
				return false
			}
		}

		return true
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

func makePath(path string, newPart any) string {
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
func diff(a, b map[string]any, path string, patch []JsonPatchOperation, strategy PatchStrategy, collections Collections) ([]JsonPatchOperation, error) {
	//TODO: handle EnsureAbsent strategy
	for key, bv := range b {
		p := makePath(path, key)
		av, ok := a[key]
		// If the key is not present in a, add it
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
		patch, err = handleValues(av, bv, p, patch, strategy, collections)
		if err != nil {
			return nil, err
		}
	}
	// Leaving this here for now, but the current thinking is that we never remove properties from objects.
	//	if strategy == PatchStrategyExactMatch {
	//		// Now add all deleted values as nil
	//		for key := range a {
	//			_, found := b[key]
	//			if !found {
	//                p := makePath(path, key)
	//				patch = append(patch, NewPatch("remove", p, nil))
	//			}
	//		}
	//	}
	return patch, nil
}

func handleValues(av, bv any, p string, patch []JsonPatchOperation, strategy PatchStrategy, collections Collections) ([]JsonPatchOperation, error) {
	var err error
	ignoreArrayOrder := !collections.isArray(p)
	switch at := av.(type) {
	case map[string]any:
		bt := bv.(map[string]any)
		patch, err = diff(at, bt, p, patch, strategy, collections)
		if err != nil {
			return nil, err
		}
		return patch, nil
	case string, float64, bool:
		if !matchesValue(av, bv, ignoreArrayOrder) {
			patch = append(patch, NewPatch("replace", p, bv))
		}
		return patch, nil
	case []any:
		bt, replaceWithOtherCollection := bv.([]any)
		switch {
		case !replaceWithOtherCollection:
			// If the types are different, we replace the whole array
			patch = append(patch, NewPatch("replace", p, bv))
		case collections.isArray(p) && len(at) != len(bt):
			patch = append(patch, compareArray(at, bt, p, strategy, collections)...)
		case collections.isArray(p) && len(at) == len(bt):
			// If arrays have the same length, we can compare them element by element
			for i := range bt {
				patch, err = handleValues(at[i], bt[i], makePath(p, i), patch, strategy, collections)
				if err != nil {
					return nil, err
				}
			}
		default:
			// If this is not an array, we treat it as a set of values.
			if !matchesValue(at, bt, true) {
				patch = append(patch, compareArray(at, bt, p, strategy, collections)...)
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
func compareArray(av, bv []any, p string, strategy PatchStrategy, collections Collections) []JsonPatchOperation {
	retval := []JsonPatchOperation{}

	switch {
	case collections.isArray(p):
		if strategy == PatchStrategyExactMatch {
			// Find elements that need to be removed
			processArray(av, bv, func(i int, value any) {
				retval = append(retval, NewPatch("remove", makePath(p, i), nil))
			}, strategy)
			reversed := make([]JsonPatchOperation, len(retval))
			for i := range retval {
				reversed[len(retval)-1-i] = retval[i]
			}
			retval = reversed
		}

		// Find elements that need to be added.
		// NOTE we pass in `bv` then `av` so that processArray can find the missing elements.
		processArray(bv, av, func(i int, value any) {
			retval = append(retval, NewPatch("add", makePath(p, i), value))
		}, strategy)
	case collections.isEntitySet(p):
		if len(av) == len(bv) && matchesValue(av, bv, true) {
			return retval
		}
		// TODO: removing is not tested yest!
		removals := 0
		if strategy == PatchStrategyExactMatch {
			// Find elements that need to be removed
			elementsBeforeRemove := len(retval)
			processIdentitySet(av, bv, p, func(i, o int, value any) {
				retval = append(retval, NewPatch("remove", makePath(p, i), nil))
			}, func(ops []JsonPatchOperation) { // no-op
			}, strategy, collections)
			removals = len(retval) - elementsBeforeRemove
			reversed := make([]JsonPatchOperation, len(retval))
			for i := range retval {
				reversed[len(retval)-1-i] = retval[i]
			}
			retval = reversed
		}
		offset := len(av) - removals
		processIdentitySet(bv, av, p, func(i, o int, value any) {
			retval = append(retval, NewPatch("add", makePath(p, o+offset), value))
		}, func(ops []JsonPatchOperation) {
			retval = append(retval, ops...)
		}, strategy, collections)
	default: // default to set
		if len(av) == len(bv) && matchesValue(av, bv, true) {
			return retval
		}
		// TODO: removing is not tested yest!
		// also we need to check for PatchStrategyEnsureAbsent
		removals := 0
		if strategy == PatchStrategyExactMatch {
			// Find elements that need to be removed
			elementsBeforeRemove := len(retval)
			processSet(av, bv, func(i int, value any) { retval = append(retval, NewPatch("remove", makePath(p, i), nil)) })
			removals = len(retval) - elementsBeforeRemove
			reversed := make([]JsonPatchOperation, len(retval))
			for i := range retval {
				reversed[len(retval)-1-i] = retval[i]
			}
			retval = reversed
		}
		offset := len(av) - removals
		processSet(bv, av, func(i int, value any) { retval = append(retval, NewPatch("add", makePath(p, i+offset), value)) })
	}

	return retval
}

func processSet(av, bv []any, applyOp func(i int, value any)) {
	foundIndexes := make(map[int]struct{}, len(av))
	lookup := make(map[string]int)

	for i, v := range bv {
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			continue // Skip if we can't marshal
		}
		jsonStr := string(jsonBytes)
		lookup[jsonStr] = i
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
		if _, ok := lookup[jsonStr]; ok {
			foundIndexes[i] = struct{}{}
		}
	}

	// Apply op for all elements in av that weren't found
	for i, v := range av {
		if _, ok := foundIndexes[i]; !ok {
			applyOp(i, v)
		}
	}
}

func processIdentitySet(av, bv []any, path string, applyOp func(i, o int, value any), replaceOps func(ops []JsonPatchOperation), strategy PatchStrategy, collections Collections) {
	foundIndexes := make(map[int]struct{}, len(av))
	lookup := make(map[string]int)

	for i, v := range bv {
		key, ok := collections.EntitySets.Get(Path(toJsonPath(path)))
		if !ok {
			continue // If we don't have a key for this path, skip
		}
		jsonBytes, err := json.Marshal(v.(map[string]any)[string(key)])
		if err != nil {
			continue // Skip if we can't marshal
		}
		jsonStr := string(jsonBytes)
		lookup[jsonStr] = i
	}

	for i, v := range av {
		key, ok := collections.EntitySets.Get(Path(toJsonPath(path)))
		if !ok {
			continue // If we don't have a key for this path, skip
		}
		jsonBytes, err := json.Marshal(v.(map[string]any)[string(key)])
		if err != nil {
			applyOp(i, 0, v) // If we can't marshal, treat it as not found
			continue
		}

		jsonStr := string(jsonBytes)
		if index, ok := lookup[jsonStr]; ok {
			foundIndexes[i] = struct{}{}
			updateOps, err := handleValues(bv[index], v, fmt.Sprintf("%s/%d", path, lookup[jsonStr]), []JsonPatchOperation{}, strategy, collections)
			if err != nil {
				return
			}
			replaceOps(updateOps)
		}
	}

	offset := 0
	for i, v := range av {
		if _, ok := foundIndexes[i]; !ok {
			applyOp(i, offset, v)
			offset++
		}
	}
}

// processArray processes `av` and `bv` calling `applyOp` whenever a value is absent.
// It keeps track of which indexes have already had `applyOp` called for and automatically skips them so you can process duplicate objects correctly.
func processArray(av, bv []any, applyOp func(i int, value any), strategy PatchStrategy) {
	foundIndexes := make(map[int]struct{}, len(av))
	switch strategy {
	case PatchStrategyExactMatch:
		reverseFoundIndexes := make(map[int]struct{}, len(bv))
		for i, v := range av {
			for i2, v2 := range bv {
				if _, ok := reverseFoundIndexes[i2]; ok {
					continue
				}
				if reflect.DeepEqual(v, v2) {
					foundIndexes[i] = struct{}{}
					reverseFoundIndexes[i2] = struct{}{}
					break
				}
			}
			if _, ok := foundIndexes[i]; !ok {
				applyOp(i, v)
			}
		}
	case PatchStrategyEnsureExists:
		offset := len(bv)
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

		for i, v := range av {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				applyOp(i+offset, v) // If we can't marshal, treat it as not found
				continue
			}

			jsonStr := string(jsonBytes)
			if bvCounts[jsonStr] > bvSeen[jsonStr] {
				foundIndexes[i] = struct{}{}
				bvSeen[jsonStr]++
			}
		}

		for i, v := range av {
			if _, ok := foundIndexes[i]; !ok {
				applyOp(i+offset, v)
			}
		}
		return
	case PatchStrategyEnsureAbsent:
		return
	}
}

func removeIgnoredFields(data any, ignoredFields []Path) (any, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	jsonStr := string(jsonBytes)

	for _, path := range ignoredFields {
		jsonStr, err = removeJSONPath(jsonStr, string(path))
		if err != nil {
			return nil, err
		}
	}

	var result any
	err = json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func removeJSONPath(jsonStr, jsonPath string) (string, error) {
	if strings.Contains(jsonPath, "[*]") {
		return removeFromArrayElements(jsonStr, jsonPath)
	}

	path := strings.TrimPrefix(jsonPath, "$.")
	path = strings.TrimPrefix(path, "$")

	result, err := sjson.Delete(jsonStr, path)
	if err != nil {
		return "", err
	}

	return result, nil
}

func removeFromArrayElements(jsonStr, jsonPath string) (string, error) {
	parts := strings.Split(jsonPath, "[*].")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid wildcard path format")
	}

	arrayPath := strings.TrimPrefix(parts[0], "$.")
	arrayPath = strings.TrimPrefix(arrayPath, "$")
	propertyToRemove := parts[1]

	arrayResult := gjson.Get(jsonStr, arrayPath)
	if !arrayResult.Exists() || !arrayResult.IsArray() {
		return jsonStr, nil
	}

	result := jsonStr
	var err error

	arrayResult.ForEach(func(key, value gjson.Result) bool {
		elementPath := fmt.Sprintf("%s.%d.%s", arrayPath, key.Int(), propertyToRemove)
		result, err = sjson.Delete(result, elementPath)
		return err == nil
	})

	return result, err
}
