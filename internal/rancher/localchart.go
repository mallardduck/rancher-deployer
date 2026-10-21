package rancher

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxChartYAMLSize bounds how much of a Chart.yaml is read from an archive.
const maxChartYAMLSize = 1 << 20

// LoadLocalChart reads Chart.yaml from a local chart directory or .tgz and
// returns the Chart to install from that path.
func LoadLocalChart(p string) (Chart, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return Chart{}, fmt.Errorf("resolving chart path %q: %w", p, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Chart{}, fmt.Errorf("chart path: %w", err)
	}

	var data []byte
	if info.IsDir() {
		data, err = os.ReadFile(filepath.Join(abs, "Chart.yaml")) //nolint:gosec // path is the user-supplied chart dir
		if err != nil {
			return Chart{}, fmt.Errorf("%s is not a chart directory: %w", abs, err)
		}
	} else {
		data, err = chartYAMLFromArchive(abs)
		if err != nil {
			return Chart{}, fmt.Errorf("reading chart archive %s: %w", abs, err)
		}
	}

	var meta struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return Chart{}, fmt.Errorf("parsing Chart.yaml in %s: %w", abs, err)
	}
	if meta.Name == "" || meta.Version == "" {
		return Chart{}, fmt.Errorf("%s: Chart.yaml must set both name and version", abs)
	}

	version := strings.TrimPrefix(meta.Version, "v")
	return Chart{
		ChartName:    meta.Name,
		Version:      version,
		IsPrerelease: strings.Contains(version, "-"),
		LocalPath:    abs,
	}, nil
}

// chartYAMLFromArchive returns the contents of the top-level Chart.yaml
// (<chart>/Chart.yaml) in a gzipped tar chart package.
func chartYAMLFromArchive(file string) ([]byte, error) {
	f, err := os.Open(file) //nolint:gosec // user-supplied chart archive
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("not a gzipped chart archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("no Chart.yaml found in archive")
		}
		if err != nil {
			return nil, err
		}
		// Only <chart>/Chart.yaml — not Chart.yaml files of bundled subcharts.
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == "Chart.yaml" && strings.Count(strings.Trim(hdr.Name, "/"), "/") == 1 {
			return io.ReadAll(io.LimitReader(tr, maxChartYAMLSize))
		}
	}
}
