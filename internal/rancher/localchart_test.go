package rancher

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

const testChartYAML = "name: rancher\nversion: v2.8.5\n"

func TestLoadLocalChart_Dir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(testChartYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLocalChart(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChartName != "rancher" || got.Version != "2.8.5" || got.IsPrerelease || got.LocalPath != dir || !got.IsLocal() {
		t.Errorf("unexpected chart: %+v", got)
	}
}

func TestLoadLocalChart_Tgz(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rancher-2.15.2-abc-head.tgz")
	f, err := os.Create(file) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	write := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	// A subchart's Chart.yaml appears first and must be ignored.
	write("rancher/charts/sub/Chart.yaml", "name: sub\nversion: 9.9.9\n")
	write("rancher/Chart.yaml", "name: rancher\nversion: 2.15.2-abc-head\n")
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()

	got, err := LoadLocalChart(file)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChartName != "rancher" || got.Version != "2.15.2-abc-head" || !got.IsPrerelease {
		t.Errorf("unexpected chart: %+v", got)
	}
}

func TestLoadLocalChart_Errors(t *testing.T) {
	if _, err := LoadLocalChart(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected error for missing path")
	}
	if _, err := LoadLocalChart(t.TempDir()); err == nil {
		t.Error("expected error for dir without Chart.yaml")
	}
}
