package bench

import (
	"context"
	"strings"
	"testing"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

func TestGoComparabilityRequiresVersionAndChecksum(t *testing.T) {
	newer, err := runSHAProfiles(context.Background(), "post", nil, func(_ context.Context, n int) ([]byte, error) {
		return fakeSHAOutput(n, shaExpectedDigest), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	original := newer
	original.Label = "historic"
	original.Go = "go1.26.10-X:nodwarf5 linux/amd64" // version in the historical i3 report
	newer.Go = "go1.26.11 linux/amd64"               // deliberately different, regardless of the test host toolchain
	diff := report.Compare(original, newer)
	if len(diff.Rows) != 3 {
		t.Fatalf("unexpected shared metrics: %+v", diff.Rows)
	}
	for _, row := range diff.Rows {
		if row.Comparable || !strings.Contains(row.Reason, "Go") {
			t.Fatalf("different Go versions must be non-comparable: %+v", row)
		}
	}
	original.Go = newer.Go
	for _, row := range report.Compare(original, newer).Rows {
		if !row.Comparable {
			t.Fatalf("same Go version and checksum should compare: %+v", row)
		}
	}
	original.GoChecksum = strings.Repeat("0", 64)
	for _, row := range report.Compare(original, newer).Rows {
		if row.Comparable {
			t.Fatalf("different checksum must be non-comparable: %+v", row)
		}
	}
}
