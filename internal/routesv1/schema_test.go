package routesv1_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestConformanceSchema checks the published JSON Schema against the
// conformance files: every valid file passes it. The schema only checks the
// shape, so invalid files may pass it too.
func TestConformanceSchema(t *testing.T) {
	b, err := os.ReadFile("../../schema/routes.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatal(err)
	}
	checkKeywords(t, schema, "schema")
	files, err := filepath.Glob("../../testdata/routes/valid/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no valid conformance files: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, p := range validate(schema, v, "$") {
			t.Errorf("%s: the schema refuses a valid file: %s", filepath.Base(f), p)
		}
	}
}

// checkKeywords fails when the schema uses a keyword validate doesn't
// implement, so the test can't pass by ignoring it.
func checkKeywords(t *testing.T, s map[string]any, at string) {
	known := map[string]bool{
		"$schema":     true,
		"$id":         true,
		"title":       true,
		"description": true,
		"type":        true,
		"const":       true,
		"required":    true,
		"properties":  true,
		"items":       true,
		"minLength":   true,
		"minimum":     true,
		"pattern":     true,
		"enum":        true,
	}
	for k, v := range s {
		if !known[k] {
			t.Errorf("%s: keyword %q isn't checked by this test's validator", at, k)
		}
		switch k {
		case "properties":
			for name, sub := range v.(map[string]any) {
				checkKeywords(t, sub.(map[string]any), at+".properties."+name)
			}
		case "items":
			checkKeywords(t, v.(map[string]any), at+".items")
		}
	}
}

// validate implements the part of JSON Schema that routes.v1.json uses.
func validate(s map[string]any, v any, at string) []string {
	var problems []string
	bad := func(msg string) { problems = append(problems, at+": "+msg) }
	if c, ok := s["const"]; ok && c != v {
		bad("want " + c.(string))
	}
	if enums, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enums {
			if e == v {
				found = true
				break
			}
		}
		if !found {
			bad("value not in enum")
		}
	}
	switch s["type"] {
	case "boolean":
		if _, ok := v.(bool); !ok {
			bad("want a boolean")
			return problems
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
			bad("want an integer")
			return problems
		}
		if min, ok := s["minimum"].(float64); ok && f < min {
			bad("below minimum")
		}
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			bad("want an object")
			return problems
		}
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, ok := obj[r.(string)]; !ok {
					bad(r.(string) + " is required")
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		for k, sub := range props {
			if x, ok := obj[k]; ok {
				problems = append(problems, validate(sub.(map[string]any), x, at+"."+k)...)
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			bad("want an array")
			return problems
		}
		for i, x := range arr {
			problems = append(problems, validate(s["items"].(map[string]any), x, at+"["+strconv.Itoa(i)+"]")...)
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			bad("want a string")
			return problems
		}
		if n, ok := s["minLength"].(float64); ok && len([]rune(str)) < int(n) {
			bad("too short")
		}
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(str) {
			bad("doesn't match " + p)
		}
	}
	return problems
}
