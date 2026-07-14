package doctor

import (
	"context"
	"testing"
	"time"
)

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		name   string
		status CheckStatus
		want   int
	}{
		{"StatusPass is 0", StatusPass, 0},
		{"StatusWarn is 1", StatusWarn, 1},
		{"StatusFail is 2", StatusFail, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.status) != tt.want {
				t.Errorf("got %d, want %d", tt.status, tt.want)
			}
		})
	}
}

func TestHasCriticalFailures(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    bool
	}{
		{
			name: "no failures",
			results: []CheckResult{
				{Status: StatusPass},
				{Status: StatusWarn},
			},
			want: false,
		},
		{
			name: "has failure",
			results: []CheckResult{
				{Status: StatusPass},
				{Status: StatusFail},
				{Status: StatusWarn},
			},
			want: true,
		},
		{
			name:    "empty results",
			results: []CheckResult{},
			want:    false,
		},
		{
			name: "all failures",
			results: []CheckResult{
				{Status: StatusFail},
				{Status: StatusFail},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasCriticalFailures(tt.results)
			if got != tt.want {
				t.Errorf("HasCriticalFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewDoctor(t *testing.T) {
	tests := []struct {
		name string
		opts *CheckOptions
	}{
		{
			name: "nil options",
			opts: nil,
		},
		{
			name: "with options",
			opts: &CheckOptions{
				Mode:           "k3d",
				Context:        ContextLocal,
				SkipNetwork:    false,
				SkipState:      false,
				NetworkTimeout: 5 * time.Second,
			},
		},
		{
			name: "skip network",
			opts: &CheckOptions{
				SkipNetwork: true,
			},
		},
		{
			name: "skip state",
			opts: &CheckOptions{
				SkipState: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDoctor(tt.opts)
			if d.opts.Context == "" {
				t.Error("Expected Context to be set to ContextLocal by default")
			}
			if d.opts.NetworkTimeout == 0 {
				t.Error("Expected NetworkTimeout to be set by default")
			}
			if len(d.checkers) == 0 {
				t.Error("Expected checkers to be registered")
			}
		})
	}
}

func TestNewDoctor_SkipFlags(t *testing.T) {
	t.Run("skip network removes network checkers", func(t *testing.T) {
		d := NewDoctor(&CheckOptions{SkipNetwork: true})

		// Count network checkers
		networkCount := 0
		for _, c := range d.checkers {
			if c.Category() == "network" {
				networkCount++
			}
		}

		if networkCount != 0 {
			t.Errorf("Expected 0 network checkers when SkipNetwork=true, got %d", networkCount)
		}
	})

	t.Run("skip state removes state checkers", func(t *testing.T) {
		d := NewDoctor(&CheckOptions{SkipState: true})

		// Count state checkers
		stateCount := 0
		for _, c := range d.checkers {
			if c.Category() == "state" {
				stateCount++
			}
		}

		if stateCount != 0 {
			t.Errorf("Expected 0 state checkers when SkipState=true, got %d", stateCount)
		}
	})
}

func TestDoctor_RunAll(t *testing.T) {
	d := NewDoctor(&CheckOptions{
		SkipNetwork: true, // Skip network to avoid actual HTTP calls
		SkipState:   true, // Skip state to avoid kubectl/helm calls
	})

	ctx := context.Background()
	results := d.RunAll(ctx)

	if len(results) == 0 {
		t.Fatal("RunAll() returned no results")
	}

	// Verify all results have required fields
	for i, r := range results {
		if r.Name == "" {
			t.Errorf("Result %d has empty Name", i)
		}
		if r.Category == "" {
			t.Errorf("Result %d has empty Category", i)
		}
		if r.Message == "" {
			t.Errorf("Result %d has empty Message", i)
		}
	}
}

func TestDoctor_RunAll_WithContext(t *testing.T) {
	d := NewDoctor(&CheckOptions{
		SkipNetwork: true,
		SkipState:   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := d.RunAll(ctx)

	if len(results) == 0 {
		t.Fatal("RunAll() returned no results")
	}
}

func TestCheckResult_Categories(t *testing.T) {
	expectedCategories := []string{"dependencies", "environment", "configuration", "network", "state"}

	// Pass minimal provider checkers to exercise the dependencies and environment categories.
	d := NewDoctor(&CheckOptions{},
		NewRequiredBinaryChecker("kubectl", "kubectl"),
		NewRuntimeChecker("k3d"),
	)
	ctx := context.Background()
	results := d.RunAll(ctx)

	foundCategories := make(map[string]bool)
	for _, r := range results {
		foundCategories[r.Category] = true
	}

	// We should have at least dependencies and environment
	if !foundCategories["dependencies"] {
		t.Error("Expected to find dependencies category")
	}
	if !foundCategories["environment"] {
		t.Error("Expected to find environment category")
	}

	// Verify all categories are in expected list
	for cat := range foundCategories {
		found := false
		for _, expected := range expectedCategories {
			if cat == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected category: %s", cat)
		}
	}
}
