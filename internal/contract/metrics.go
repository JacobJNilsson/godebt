package contract

// metricNames are the metric ids of spec 002 without the source set, and a report test proves that the measures emit exactly these.
var metricNames = []string{
	"complexity.cc.max", "complexity.cc.p50", "complexity.cc.p90", "complexity.functions",
	"duplication.density", "duplication.duplicated-lines", "duplication.groups",
	"erosion.eroded-count", "erosion.eroded-share", "erosion.excess-mass", "erosion.excess-share", "erosion.mass",
}

// MetricIDs returns every metric id that a report holds, for both measured sets, in sorted order.
func MetricIDs() []string {
	ids := make([]string, 0, 2*len(metricNames))
	for _, name := range metricNames {
		ids = append(ids, name+"."+string(Production), name+"."+string(Test))
	}
	return ids
}
