package routesv1_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/epistemer/handlers/internal/routesv1"
)

func TestConformanceValid(t *testing.T) {
	files, err := filepath.Glob("../../testdata/routes/valid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no valid conformance files found")
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", file, err)
			}
			var v any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", file, err)
			}
			if err := routesv1.Check(v); err != nil {
				t.Fatalf("routesv1.Check(%s) unexpected error: %v", file, err)
			}
		})
	}
}

func TestConformanceInvalid(t *testing.T) {
	jsonFiles, err := filepath.Glob("../../testdata/routes/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonFiles) == 0 {
		t.Fatal("no invalid conformance files found")
	}

	errFiles, err := filepath.Glob("../../testdata/routes/invalid/*.err")
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonFiles) != len(errFiles) {
		t.Fatalf("mismatched count: %d .json files vs %d .err files", len(jsonFiles), len(errFiles))
	}

	for _, file := range jsonFiles {
		name := filepath.Base(file)
		testName := strings.TrimSuffix(name, ".json")
		errFile := filepath.Join("../../testdata/routes/invalid", testName+".err")

		t.Run(testName, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", file, err)
			}
			wantErrBytes, err := os.ReadFile(errFile)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", errFile, err)
			}
			wantErr := strings.TrimRight(string(wantErrBytes), "\r\n")

			var v any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", file, err)
			}

			checkErr := routesv1.Check(v)
			if checkErr == nil {
				t.Fatalf("routesv1.Check(%s) succeeded, want error %q", file, wantErr)
			}

			if checkErr.Error() != wantErr {
				t.Fatalf("routesv1.Check(%s):\ngot:  %q\nwant: %q", file, checkErr.Error(), wantErr)
			}
		})
	}
}
