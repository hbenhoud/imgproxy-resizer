// Package config loads the YAML configuration: imgproxy profiles and
// variant presets offered by the UI.
package config

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"imgproxy-resizer/internal/imgproxy"
)

const DefaultListen = "127.0.0.1:8099"

type Config struct {
	Listen         string                   `yaml:"listen"`
	DefaultProfile string                   `yaml:"default_profile"`
	Profiles       map[string]ProfileConfig `yaml:"profiles"`
	Presets        []Preset                 `yaml:"presets"`
}

type ProfileConfig struct {
	Host     string `yaml:"host"`
	Key      string `yaml:"key"`
	Salt     string `yaml:"salt"`
	Insecure bool   `yaml:"insecure"`
}

// Preset is a named set of variants the UI can prefill in one click.
type Preset struct {
	Name     string          `yaml:"name" json:"name"`
	Variants []PresetVariant `yaml:"variants" json:"variants"`
}

// PresetVariant mirrors imgproxy.Variant with YAML-friendly names and the
// max weight in KB, as typed in the UI.
type PresetVariant struct {
	Label   string  `yaml:"label" json:"label"`
	Resize  string  `yaml:"resize" json:"resizeType"`
	Width   int     `yaml:"width" json:"width"`
	Height  int     `yaml:"height" json:"height"`
	Enlarge bool    `yaml:"enlarge" json:"enlarge"`
	Extend  bool    `yaml:"extend" json:"extend"`
	Quality int     `yaml:"quality" json:"quality"`
	MaxKB   int     `yaml:"max_kb" json:"maxKB"`
	Sharpen float64 `yaml:"sharpen" json:"sharpen"`
	Extra   string  `yaml:"extra" json:"extra"`
}

// Load reads the file, expands ${ENV} references and unmarshals it.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse parses YAML content after environment expansion.
func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(raw))), &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if len(c.Profiles) == 0 {
		return nil, errors.New("config: no profiles defined")
	}
	for i, p := range c.Presets {
		if p.Name == "" {
			return nil, fmt.Errorf("config: preset #%d has no name", i+1)
		}
		for j, v := range p.Variants {
			if v.Resize == "" {
				c.Presets[i].Variants[j].Resize = "force"
			}
		}
	}
	return &c, nil
}

// Build builds the usable imgproxy profiles. Profiles that fail validation
// (typically a missing key because its env var is unset) are returned in
// skipped with the reason, so the caller can warn instead of aborting.
func (c *Config) Build() (ok map[string]imgproxy.Profile, skipped map[string]error) {
	ok = map[string]imgproxy.Profile{}
	skipped = map[string]error{}
	for name, pc := range c.Profiles {
		p, err := imgproxy.NewProfile(pc.Host, pc.Key, pc.Salt, pc.Insecure)
		if err != nil {
			skipped[name] = err
			continue
		}
		ok[name] = p
	}
	return ok, skipped
}

// Names returns profile names sorted for stable display.
func Names(m map[string]imgproxy.Profile) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
