// Package godebt measures the structural debt of a Go module.
package godebt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/JacobJNilsson/godebt/internal/compare"
	"github.com/JacobJNilsson/godebt/internal/complexity"
	"github.com/JacobJNilsson/godebt/internal/config"
	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/duplication"
	"github.com/JacobJNilsson/godebt/internal/inventory"
	"github.com/JacobJNilsson/godebt/internal/policy"
	"github.com/JacobJNilsson/godebt/internal/report"
	"github.com/JacobJNilsson/godebt/internal/safeguards"
)

// Version is the godebt release that produced a report.
const Version = "0.4.0"

// Report is the result of one audit.
type Report = report.Report

// Config is the content of one godebt.yaml.
type Config = config.File

// Policy is the policy block of a Config.
type Policy = config.Policy

// Comparison is the result of Compare.
type Comparison = compare.Comparison

// Reason is one failed policy check.
type Reason = policy.Reason

// PolicyResult is the result of Evaluate.
type PolicyResult = policy.Result

// Options holds the configuration of an audit.
type Options struct {
	// Config is the configuration of the audit, and nil reads godebt.yaml at the audited root when it exists.
	Config *Config
	// Omit is the path of one file that the audit leaves out of the walk and the coverage counts, and "" omits none.
	Omit string
}

// Audit measures the Go module at root and evaluates no policy.
// A parse error is part of the report and not an error.
func Audit(ctx context.Context, root string, opts Options) (*Report, error) {
	start := time.Now()
	cfg := opts.Config
	if cfg == nil {
		var err error
		if cfg, err = LoadDefaultConfig(root); err != nil {
			return nil, err
		}
	}
	measured := cfg.Contract()
	tree, err := discover.Walk(ctx, root, discover.Options{Exclude: measured.Exclude, TestPatterns: measured.TestPatterns, Omit: opts.Omit})
	if err != nil {
		return nil, err
	}
	inv := inventory.Build(tree)
	metrics, findings := complexity.Measure(inv)
	dupMetrics, clones, limits := duplication.Measure(inv)
	for id, m := range dupMetrics {
		metrics[id] = m
	}
	findings = append(findings, clones...)
	guards, broken := safeguards.Inspect(tree)
	findings = append(findings, broken...)
	contract.SortFindings(findings)
	run := report.Run{AnalyzerVersion: Version, Config: measured}
	r := report.New(inv, metrics, findings, limits, guards, run)
	r.Meta.DurationMs = time.Since(start).Milliseconds()
	return r, nil
}

// Baseline audits like Audit and returns the reproducible JSON form of spec 003, the bytes of a committed baseline file.
func Baseline(ctx context.Context, root string, opts Options) ([]byte, error) {
	r, err := Audit(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	r.Repo.Root = "."
	r.Meta.DurationMs = 0
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LoadConfig reads and validates the configuration file at path, and a missing file is an error.
func LoadConfig(path string) (*Config, error) {
	return config.Load(path)
}

// LoadDefaultConfig reads godebt.yaml at root, and returns nil and no error when root has none.
func LoadDefaultConfig(root string) (*Config, error) {
	return config.LoadDefault(root)
}

// LoadReport reads a saved JSON report and rejects one that breaks the contract of spec 003.
func LoadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := report.Load(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// Compare compares two reports, and refuses inside the result when their versions or configuration digests differ.
func Compare(before, after *Report) *Comparison {
	return compare.Compare(before, after)
}

// Evaluate runs policy p against r, and a nil baseline skips the checks that need one.
func Evaluate(p Policy, r *Report, baseline *Report) PolicyResult {
	var cmp *Comparison
	if baseline != nil {
		cmp = Compare(baseline, r)
	}
	return EvaluateWithComparison(p, r, cmp)
}

// EvaluateWithComparison runs policy p against r with a comparison the caller already made, and a nil cmp means no baseline.
func EvaluateWithComparison(p Policy, r *Report, cmp *Comparison) PolicyResult {
	return policy.Evaluate(p, r, cmp)
}
