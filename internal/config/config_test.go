package config

import (
	"testing"
)

func TestParseExpandsEnvAndSkipsInvalid(t *testing.T) {
	t.Setenv("IR_TEST_KEY", "736563726574")
	t.Setenv("IR_TEST_SALT", "73616c7479")
	t.Setenv("IR_UNSET_KEY", "")
	raw := []byte(`
default_profile: production
profiles:
  production:
    host: https://imgproxy.example.com
    key: ${IR_TEST_KEY}
    salt: ${IR_TEST_SALT}
  staging:
    host: https://imgproxy-staging.example.com
    key: ${IR_UNSET_KEY}
    salt: ${IR_UNSET_KEY}
  local:
    host: http://localhost:8080
    insecure: true
presets:
  - name: Thumbnails
    variants:
      - {label: small, width: 320, height: 180, max_kb: 40}
      - {label: large, resize: fit, width: 1280, height: 720, enlarge: true, quality: 85}
`)
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != DefaultListen {
		t.Fatalf("listen = %q, want default", c.Listen)
	}
	if c.DefaultProfile != "production" {
		t.Fatalf("default_profile = %q", c.DefaultProfile)
	}
	ok, skipped := c.Build()
	if _, found := ok["production"]; !found {
		t.Error("production should be valid")
	}
	if !ok["local"].Insecure {
		t.Error("local should be insecure")
	}
	if _, found := skipped["staging"]; !found {
		t.Error("staging should be skipped (empty key)")
	}
	if got := Names(ok); len(got) != 2 || got[0] != "local" || got[1] != "production" {
		t.Errorf("Names = %v", got)
	}
	if len(c.Presets) != 1 || c.Presets[0].Name != "Thumbnails" || len(c.Presets[0].Variants) != 2 {
		t.Fatalf("presets = %+v", c.Presets)
	}
	small, large := c.Presets[0].Variants[0], c.Presets[0].Variants[1]
	if small.Resize != "force" || small.MaxKB != 40 {
		t.Errorf("small = %+v (resize should default to force)", small)
	}
	if large.Resize != "fit" || !large.Enlarge || large.Quality != 85 {
		t.Errorf("large = %+v", large)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("listen: x\n")); err == nil {
		t.Error("expected error for missing profiles")
	}
	if _, err := Parse([]byte("profiles: [\n")); err == nil {
		t.Error("expected YAML error")
	}
	if _, err := Parse([]byte("profiles: {a: {host: http://x, insecure: true}}\npresets: [{variants: []}]\n")); err == nil {
		t.Error("expected error for unnamed preset")
	}
}
