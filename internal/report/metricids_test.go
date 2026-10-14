package report

import (
	"reflect"
	"sort"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// TestMetricIDsMatchTheMeasures asserts that contract.MetricIDs lists exactly the ids that the measures emit.
func TestMetricIDsMatchTheMeasures(t *testing.T) {
	r := newReport(t, "functions")
	var emitted []string
	for id := range r.Metrics {
		emitted = append(emitted, id)
	}
	sort.Strings(emitted)
	if got := contract.MetricIDs(); !reflect.DeepEqual(got, emitted) {
		t.Errorf("contract.MetricIDs() = %v, want the emitted ids %v", got, emitted)
	}
}
