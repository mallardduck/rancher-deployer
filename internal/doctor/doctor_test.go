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
			if d == nil {
				t.Fatal("NewDoctor() returned nil")
			}
			if d.opts == nil {
				t.Fatal("Doctor.opts is nil")
			}
			if d.checkers == nil {
				t.Fatal("Doctor.checkers is nil")
			}

			// Verify default context is set
			if d.opts.Context == "" {
				t.Error("Expected Context to be set to ContextLocal by default")
			}

			// Verify default network timeout is set
			if d.opts.NetworkTimeout == 0 {
				t.Error("Expected NetworkTimeout to be set by default")
			}

			// Verify checkers are registered
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

func TestDoctor_ModeSpecificCheckers(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		expectK3sCheck  bool
		expectK3dCheck  bool
		expectContainer bool
	}{
		{
			name:            "k3s mode",
			mode:            "k3s",
			expectK3sCheck:  true,
			expectK3dCheck:  false,
			expectContainer: false,
		},
		{
			name:            "k3d mode",
			mode:            "k3d",
			expectK3sCheck:  false,
			expectK3dCheck:  true,
			expectContainer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDoctor(&CheckOptions{
				Mode:        tt.mode,
				SkipNetwork: true,
				SkipState:   true,
			})

			hasK3s := false
			hasK3d := false
			hasContainer := false

			for _, c := range d.checkers {
				if c.Name() == "k3s" {
					hasK3s = true
				}
				if c.Name() == "k3d" {
					hasK3d = true
				}
				if c.Name() == "container runtime" {
					hasContainer = true
				}
			}

			if hasK3s != tt.expectK3sCheck {
				t.Errorf("Expected k3s checker = %v, got %v", tt.expectK3sCheck, hasK3s)
			}
			if hasK3d != tt.expectK3dCheck {
				t.Errorf("Expected k3d checker = %v, got %v", tt.expectK3dCheck, hasK3d)
			}
			if hasContainer != tt.expectContainer {
				t.Errorf("Expected container checker = %v, got %v", tt.expectContainer, hasContainer)
			}
		})
	}
}

func TestCheckResult_Categories(t *testing.T) {
	expectedCategories := []string{"dependencies", "environment", "configuration", "network", "state"}

	d := NewDoctor(&CheckOptions{})
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
