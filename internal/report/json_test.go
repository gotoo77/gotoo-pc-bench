package report

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func legacy(t *testing.T) Run {
	t.Helper()
	r, err := Parse("rapport_before.txt", strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func jsonDoc(t *testing.T, r Run) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestJSONRoundTripAndPrivacy(t *testing.T) {
	r := legacy(t)
	r.ID = "rapport_before.json"
	data := jsonDoc(t, r)
	if bytes.Contains(data, []byte("sensitive-do-not-publish")) || bytes.Contains(data, []byte("/private/path")) {
		t.Fatal("legacy machine ID or file paths leaked")
	}
	actual, err := ParseJSON(r.ID, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(actual.Metrics) != len(r.Metrics) || actual.CPU != r.CPU || actual.VideoSHA256 != r.VideoSHA256 || actual.GoChecksum != r.GoChecksum || !actual.Complete {
		t.Fatalf("roundtrip mismatch: %+v", actual)
	}
	if len(Compare(actual, actual).Rows) != len(r.Metrics) {
		t.Fatal("metrics not comparable after roundtrip")
	}
}

func TestJSONRejectsInvalidDocuments(t *testing.T) {
	r := legacy(t)
	r.ID = "rapport_before.json"
	good := jsonDoc(t, r)
	cases := []struct {
		name    string
		payload []byte
	}{
		{"unknown schema", bytes.Replace(good, []byte(`"schema_version": 1`), []byte(`"schema_version": 2`), 1)},
		{"missing schema", bytes.Replace(good, []byte(`"schema_version": 1,`), []byte(``), 1)},
		{"unrecognized field", bytes.Replace(good, []byte(`"schema_version": 1`), []byte(`"unexpected": true, "schema_version": 1`), 1)},
		{"bad checksum", bytes.Replace(good, []byte(r.VideoSHA256), []byte("not-a-sha"), 1)},
		{"negative value", bytes.Replace(good, []byte(`"value": 1117.78`), []byte(`"value": -3`), 1)},
		{"duplicate metric", []byte(`{"schema_version":1,"run":{"id":"rapport_before.json","label":"before","complete":true,"metrics":[{"key":"cpu.1","label":"cpu","value":1,"unit":"événements/s","higher_is_better":true},{"key":"cpu.1","label":"cpu","value":2,"unit":"événements/s","higher_is_better":true}]}}`)},
		{"duplicate JSON field", bytes.Replace(good, []byte(`"schema_version": 1`), []byte(`"schema_version": 1, "schema_version": 1`), 1)},
		{"missing direction", bytes.Replace(good, []byte(`"higher_is_better": true`), []byte(``), 1)},
		{"trailing document", append(append([]byte(nil), good...), []byte(`{}`)...)},
		{"corrupted", []byte(`{"schema_version":1`)},
		{"oversize", bytes.Repeat([]byte("x"), maxReportSize+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseJSON(r.ID, bytes.NewReader(tc.payload)); err == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
	if _, err := ParseJSON("rapport_other.json", bytes.NewReader(good)); err == nil {
		t.Fatal("spoofed filename accepted")
	}
}

func TestWriteRejectsUnsafeMetadataAndMetric(t *testing.T) {
	r := legacy(t)
	r.ID = "rapport_before.json"
	r.Metrics[0].Unit = "arbitrary"
	if err := WriteJSON(io.Discard, r); err == nil {
		t.Fatal("unit mismatch accepted")
	}
	r = legacy(t)
	r.ID = "rapport_before.json"
	r.Label = "../outside"
	if err := WriteJSON(io.Discard, r); err == nil {
		t.Fatal("unsafe label accepted")
	}
}

func TestExportAndLoadDedupe(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "rapport_before.txt")
	if err := os.WriteFile(source, []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := ExportLegacy(dir, "rapport_before.txt")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(target) != "rapport_before.json" {
		t.Fatal(target)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode().Perm())
	}
	runs, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "rapport_before.json" {
		t.Fatalf("not deduplicated: %+v", runs)
	}
	if _, err := ExportLegacy(dir, "rapport_before.txt"); err == nil {
		t.Fatal("overwrote existing JSON")
	}
	if _, err := ExportLegacy(dir, "../rapport_before.txt"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := os.WriteFile(target, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	runs, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "rapport_before.txt" {
		t.Fatalf("lost legacy fallback: %+v", runs)
	}
}

func TestExportRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(other, []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside.txt", filepath.Join(dir, "rapport_link.txt")); err != nil {
		t.Skip(err)
	}
	if _, err := ExportLegacy(dir, "rapport_link.txt"); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestCompareRefusesMismatchedMetricSemantics(t *testing.T) {
	before := legacy(t)
	after := before
	after.Metrics = append([]Metric(nil), before.Metrics...)
	after.Metrics[0].Unit = "s"
	for _, row := range Compare(before, after).Rows {
		if row.Key == after.Metrics[0].Key && row.Comparable {
			t.Fatal("different metric semantics accepted")
		}
	}
}

func TestJSONMissingRequiredComplete(t *testing.T) {
	r := legacy(t)
	r.ID = "rapport_before.json"
	data := jsonDoc(t, r)
	var env map[string]interface{}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	run := env["run"].(map[string]interface{})
	delete(run, "complete")
	corrupted, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseJSON(r.ID, bytes.NewReader(corrupted)); err == nil {
		t.Fatal("missing complete accepted")
	}
}
