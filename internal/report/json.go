package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SchemaVersion is the on-disk JSON contract, not a benchmark protocol version.
const SchemaVersion = 1

type Document struct {
	SchemaVersion int `json:"schema_version"`
	Run           Run `json:"run"`
}

var (
	reportID = regexp.MustCompile(`^rapport_[A-Za-z0-9_-]+\.(txt|json)$`)
	metricID = regexp.MustCompile(`^(cpu|ram|ffmpeg|go)\.[1-9][0-9]*$`)
	sha256ID = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidateRun checks the structural and numeric contract for persisted runs.
// It cannot certify that a benchmark was executed honestly or comparably.
func ValidateRun(r Run) error {
	if !reportID.MatchString(r.ID) || len(r.ID) > 255 {
		return errors.New("invalid report id")
	}
	if !safeLabel.MatchString(r.Label) || len(r.Label) > 128 {
		return errors.New("invalid label")
	}
	if r.Timestamp != "" {
		if _, err := time.Parse(time.RFC3339, r.Timestamp); err != nil {
			return fmt.Errorf("invalid timestamp: %w", err)
		}
	}
	for _, s := range []string{r.OS, r.Kernel, r.CPU, r.Cores, r.Threads, r.RAM, r.Board, r.BIOS, r.FFmpeg, r.Sysbench, r.Go, r.VideoSpec} {
		if len(s) > 512 {
			return errors.New("metadata field too long")
		}
	}
	for _, sha := range []string{r.VideoSHA256, r.GoChecksum} {
		if sha != "" && !sha256ID.MatchString(sha) {
			return errors.New("invalid SHA-256 checksum")
		}
	}
	if len(r.Metrics) == 0 || len(r.Metrics) > 64 {
		return errors.New("expected 1..64 metrics")
	}
	if len(r.Warnings) > 32 {
		return errors.New("too many warnings")
	}
	for _, s := range r.Warnings {
		if len(s) > 512 {
			return errors.New("warning too long")
		}
	}
	seen := make(map[string]bool, len(r.Metrics))
	for _, m := range r.Metrics {
		if !metricID.MatchString(m.Key) || seen[m.Key] || len(m.Label) == 0 || len(m.Label) > 128 || math.IsNaN(m.Value) || math.IsInf(m.Value, 0) || m.Value <= 0 {
			return fmt.Errorf("invalid or duplicate metric: %q", m.Key)
		}
		seen[m.Key] = true
		switch strings.SplitN(m.Key, ".", 2)[0] {
		case "cpu":
			if m.Unit != "événements/s" || !m.Higher {
				return fmt.Errorf("invalid CPU metric units/direction: %s", m.Key)
			}
		case "ram":
			if m.Unit != "MiB/s" || !m.Higher {
				return fmt.Errorf("invalid RAM metric units/direction: %s", m.Key)
			}
		case "ffmpeg", "go":
			if m.Unit != "s" || m.Higher {
				return fmt.Errorf("invalid timing metric units/direction: %s", m.Key)
			}
		}
	}
	return nil
}

// WriteJSON emits a validated, versioned document to a caller-owned writer.
func WriteJSON(w io.Writer, r Run) error {
	if err := ValidateRun(r); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Document{SchemaVersion: SchemaVersion, Run: r})
}

func hasRequired(raw map[string]json.RawMessage, names ...string) error {
	for _, name := range names {
		v, ok := raw[name]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("required field missing: %s", name)
		}
	}
	return nil
}

// rejectDuplicateFields prevents differing JSON implementations from assigning
// conflicting meanings to the same object member. It accepts nested arrays.
func rejectDuplicateFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, isDelim := tok.(json.Delim)
		if !isDelim {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				name, ok := k.(string)
				if !ok {
					return errors.New("invalid JSON object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate JSON field: %s", name)
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	return walk()
}

// ParseJSON requires the v1 schema, bounds input size and rejects unknown fields.
// The document ID must equal its filename to prevent provenance spoofing.
func ParseJSON(id string, reader io.Reader) (Run, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxReportSize+1))
	if err != nil {
		return Run{}, err
	}
	if len(data) > maxReportSize {
		return Run{}, errors.New("JSON report exceeds size limit")
	}
	if err := rejectDuplicateFields(data); err != nil {
		return Run{}, err
	}
	var doc Document
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return Run{}, fmt.Errorf("decode report JSON: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return Run{}, errors.New("JSON must contain exactly one document")
	}
	if doc.SchemaVersion != SchemaVersion {
		return Run{}, fmt.Errorf("unsupported schema_version: %d", doc.SchemaVersion)
	}
	// Explicit required-field checks avoid confusing missing booleans with false.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Run{}, err
	}
	if err := hasRequired(envelope, "schema_version", "run"); err != nil {
		return Run{}, err
	}
	var rawRun map[string]json.RawMessage
	if err := json.Unmarshal(envelope["run"], &rawRun); err != nil {
		return Run{}, err
	}
	if err := hasRequired(rawRun, "id", "label", "complete", "metrics"); err != nil {
		return Run{}, err
	}
	var rawMetrics []map[string]json.RawMessage
	if err := json.Unmarshal(rawRun["metrics"], &rawMetrics); err != nil {
		return Run{}, err
	}
	for i, m := range rawMetrics {
		if err := hasRequired(m, "key", "label", "value", "unit", "higher_is_better"); err != nil {
			return Run{}, fmt.Errorf("metric %d: %w", i, err)
		}
	}
	if doc.Run.ID != id {
		return Run{}, fmt.Errorf("report ID %q differs from filename %q", doc.Run.ID, id)
	}
	if err := ValidateRun(doc.Run); err != nil {
		return Run{}, err
	}
	return doc.Run, nil
}

// ExportLegacy creates a sanitized JSON sibling of a regular legacy report.
// It neither overwrites existing files nor follows symlinks.
func ExportLegacy(dir, name string) (string, error) {
	if !reportID.MatchString(name) || !strings.HasSuffix(name, ".txt") {
		return "", errors.New("--from must be a rapport_*.txt basename")
	}
	path := filepath.Join(dir, name)
	st, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Size() < 1 || st.Size() > maxReportSize {
		return "", errors.New("source is not a valid regular report file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	r, parseErr := Parse(name, f)
	closeErr := f.Close()
	if parseErr != nil {
		return "", parseErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	target := strings.TrimSuffix(name, ".txt") + ".json"
	r.ID = target
	if err := ValidateRun(r); err != nil {
		return "", fmt.Errorf("legacy report cannot be exported: %w", err)
	}
	outPath := filepath.Join(dir, target)
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	writeErr := WriteJSON(out, r)
	closeErr = out.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(outPath)
		return "", errors.Join(writeErr, closeErr)
	}
	return outPath, nil
}
