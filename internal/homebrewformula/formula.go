package homebrewformula

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type Asset struct {
	GOOS   string
	GOARCH string
	Name   string
	SHA256 string
	URL    string
}

type Formula struct {
	Tag       string
	Version   string
	Repo      string
	ClassName string
	Desc      string
	Homepage  string
	Assets    []Asset
}

var supportedPairs = []struct {
	goos   string
	goarch string
}{
	{goos: "darwin", goarch: "amd64"},
	{goos: "darwin", goarch: "arm64"},
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
}

func Build(tag, repo, checksumsPath, downloadBase string) (Formula, error) {
	tag = normalizeTag(tag)
	if tag == "" {
		return Formula{}, fmt.Errorf("homebrew formula requires a release tag")
	}
	if strings.TrimSpace(repo) == "" {
		return Formula{}, fmt.Errorf("homebrew formula requires repository owner/repo")
	}
	if strings.TrimSpace(checksumsPath) == "" {
		return Formula{}, fmt.Errorf("homebrew formula requires checksums.txt path")
	}
	if strings.TrimSpace(downloadBase) == "" {
		return Formula{}, fmt.Errorf("homebrew formula requires download base URL")
	}

	sumByAsset, err := parseChecksums(checksumsPath)
	if err != nil {
		return Formula{}, err
	}

	version := strings.TrimPrefix(tag, "v")
	assets := make([]Asset, 0, len(supportedPairs))
	for _, pair := range supportedPairs {
		name := fmt.Sprintf("plugin-kit-ai_%s_%s_%s.tar.gz", version, pair.goos, pair.goarch)
		sum, ok := sumByAsset[name]
		if !ok {
			return Formula{}, fmt.Errorf("checksums.txt missing asset %q", name)
		}
		assets = append(assets, Asset{
			GOOS:   pair.goos,
			GOARCH: pair.goarch,
			Name:   name,
			SHA256: sum,
			URL:    strings.TrimRight(downloadBase, "/") + "/" + name,
		})
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].GOOS == assets[j].GOOS {
			return assets[i].GOARCH < assets[j].GOARCH
		}
		return assets[i].GOOS < assets[j].GOOS
	})

	return Formula{
		Tag:       tag,
		Version:   version,
		Repo:      repo,
		ClassName: "PluginKitAi",
		Desc:      "AI CLI plugin runtime with a first-class Go SDK",
		Homepage:  "https://github.com/" + repo,
		Assets:    assets,
	}, nil
}

func Render(f Formula) ([]byte, error) {
	const tpl = `class {{ .ClassName }} < Formula
  desc "{{ .Desc }}"
  homepage "{{ .Homepage }}"
  version "{{ .Version }}"

  on_macos do
    if Hardware::CPU.arm?
      url "{{ assetURL .Assets "darwin" "arm64" }}"
      sha256 "{{ assetSHA .Assets "darwin" "arm64" }}"
    else
      url "{{ assetURL .Assets "darwin" "amd64" }}"
      sha256 "{{ assetSHA .Assets "darwin" "amd64" }}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "{{ assetURL .Assets "linux" "arm64" }}"
      sha256 "{{ assetSHA .Assets "linux" "arm64" }}"
    else
      url "{{ assetURL .Assets "linux" "amd64" }}"
      sha256 "{{ assetSHA .Assets "linux" "amd64" }}"
    end
  end

  def install
    bin.install "plugin-kit-ai"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/plugin-kit-ai version")
  end
end
`

	t, err := template.New("formula").Funcs(template.FuncMap{
		"assetURL": func(assets []Asset, goos, goarch string) string {
			return mustAsset(assets, goos, goarch).URL
		},
		"assetSHA": func(assets []Asset, goos, goarch string) string {
			return mustAsset(assets, goos, goarch).SHA256
		},
	}).Parse(tpl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Write(outputPath string, body []byte) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("homebrew formula output path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, body, 0o644)
}

func parseChecksums(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("invalid checksums.txt line %q", line)
		}
		sum := strings.TrimSpace(fields[0])
		name := strings.TrimPrefix(strings.TrimSpace(fields[len(fields)-1]), "*")
		out[name] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func mustAsset(assets []Asset, goos, goarch string) Asset {
	for _, asset := range assets {
		if asset.GOOS == goos && asset.GOARCH == goarch {
			return asset
		}
	}
	panic(fmt.Sprintf("missing asset for %s/%s", goos, goarch))
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}
