package imgproxy

import (
	"strings"
	"testing"
)

// Dummy hex key/salt: "secret" / "salty".
const (
	testKey  = "736563726574"
	testSalt = "73616c7479"
	testHost = "https://imgproxy.example.com"
)

// Golden signed URL for the key/salt above. The signature was accepted by
// imgproxy v3.20.0 running with IMGPROXY_KEY=736563726574 and
// IMGPROXY_SALT=73616c7479 (a tampered signature is rejected with 403).
const goldenURL = "https://imgproxy.example.com/qv0zNoXPcNzvIznoYpocQBpfnJQNKeCqTQjOeHMLPwA/rs:fit:300:300/plain/https://imgsource.com/photo.jpg@jpeg"

func testProfile(t *testing.T) Profile {
	t.Helper()
	p, err := NewProfile(testHost, testKey, testSalt, false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSignGoldenValue(t *testing.T) {
	p := testProfile(t)
	got := p.URL("/rs:fit:300:300/plain/https://imgsource.com/photo.jpg@jpeg")
	if got != goldenURL {
		t.Fatalf("signature mismatch\n got %s\nwant %s", got, goldenURL)
	}
}

func TestBuildURLInline(t *testing.T) {
	p := testProfile(t)
	v := Variant{ResizeType: "force", Width: 1640, Height: 616, Enlarge: true, MaxBytes: 153600}
	got, err := p.BuildURL(v, "https://imgsource.com/photo.jpg", "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "/rs:force:1640:616:1/mb:153600/plain/https://imgsource.com/photo.jpg@jpeg"
	if !strings.HasSuffix(got, wantPath) {
		t.Fatalf("got %s, want suffix %s", got, wantPath)
	}
	if !strings.HasPrefix(got, testHost+"/") || strings.Contains(got, "/insecure/") {
		t.Fatalf("got %s, want signed URL on %s", got, testHost)
	}
	sig := strings.TrimSuffix(strings.TrimPrefix(got, testHost+"/"), wantPath)
	if sig != Sign(p.Key, p.Salt, wantPath) {
		t.Fatalf("signature %q does not match Sign()", sig)
	}
}

func TestInsecureProfile(t *testing.T) {
	p, err := NewProfile("http://localhost:8080/", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	got := p.URL("/rs:fit:100:100/plain/https://a.b/c.png@png")
	want := "http://localhost:8080/insecure/rs:fit:100:100/plain/https://a.b/c.png@png"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestNewProfileErrors(t *testing.T) {
	cases := []struct{ host, key, salt string }{
		{"", testKey, testSalt},
		{"imgproxy.example.com", testKey, testSalt},
		{testHost, "", testSalt},
		{testHost, "zz", testSalt},
		{testHost, testKey, ""},
	}
	for _, c := range cases {
		if _, err := NewProfile(c.host, c.key, c.salt, false); err == nil {
			t.Errorf("NewProfile(%q,%q,%q) expected error", c.host, c.key, c.salt)
		}
	}
}

func TestEscapeSource(t *testing.T) {
	cases := map[string]string{
		"https://imgsource.com/photo.jpg":          "https://imgsource.com/photo.jpg",
		"https://imgsource.com/photo.jpg?v=2&x=1":  "https://imgsource.com/photo.jpg%3Fv=2&x=1",
		"https://user@imgsource.com/a@b.jpg":       "https://user%40imgsource.com/a%40b.jpg",
		"  https://imgsource.com/with space.jpg  ": "https://imgsource.com/with%20space.jpg",
	}
	for in, want := range cases {
		got, err := EscapeSource(in)
		if err != nil {
			t.Errorf("EscapeSource(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("EscapeSource(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "photo.jpg", "/local/photo.jpg", "ftp://x/y.jpg", "https://"} {
		if _, err := EscapeSource(bad); err == nil {
			t.Errorf("EscapeSource(%q) expected error", bad)
		}
	}
}

func TestBuildPathExt(t *testing.T) {
	if _, err := BuildPath("rs:fit:1:1", "https://a.b/c.png", "bmp"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
	got, err := BuildPath("/rs:fit:1:1/", "https://a.b/c.png", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/rs:fit:1:1/plain/https://a.b/c.png" {
		t.Fatalf("got %s", got)
	}
}

func TestVariantOptions(t *testing.T) {
	cases := []struct {
		name string
		v    Variant
		want string
	}{
		{"force plain", Variant{ResizeType: "force", Width: 820, Height: 306}, "rs:force:820:306"},
		{"with mb", Variant{ResizeType: "force", Width: 820, Height: 306, MaxBytes: 102400}, "rs:force:820:306/mb:102400"},
		{"enlarge", Variant{ResizeType: "force", Width: 1640, Height: 616, Enlarge: true}, "rs:force:1640:616:1"},
		{"extend only", Variant{ResizeType: "fit", Width: 100, Height: 100, Extend: true}, "rs:fit:100:100:0:1"},
		{"enlarge+extend", Variant{ResizeType: "fit", Width: 100, Height: 100, Enlarge: true, Extend: true}, "rs:fit:100:100:1:1"},
		{"all", Variant{ResizeType: "fill", Width: 350, Height: 109, Enlarge: true, Quality: 92, MaxBytes: 122880, Sharpen: 0.4, Extra: "/bl:0.5/dpr:2/"},
			"rs:fill:350:109:1/q:92/mb:122880/sh:0.4/bl:0.5/dpr:2"},
		{"width only", Variant{ResizeType: "auto", Width: 500}, "rs:auto:500:0"},
	}
	for _, c := range cases {
		got, err := c.v.Options()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestVariantValidate(t *testing.T) {
	bad := []Variant{
		{ResizeType: "stretch", Width: 1, Height: 1},
		{ResizeType: "force"},
		{ResizeType: "force", Width: -1, Height: 1},
		{ResizeType: "force", Width: 1, Height: 1, Quality: 101},
		{ResizeType: "force", Width: 1, Height: 1, MaxBytes: -5},
		{ResizeType: "force", Width: 1, Height: 1, Sharpen: -1},
		{ResizeType: "force", Width: 1, Height: 1, Extra: "plain"},
		{ResizeType: "force", Width: 1, Height: 1, Extra: "bl:0.5/x y:1"},
	}
	for i, v := range bad {
		if err := v.Validate(); err == nil {
			t.Errorf("case %d: expected error for %+v", i, v)
		}
	}
}
