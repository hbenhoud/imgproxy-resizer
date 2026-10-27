package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"imgproxy-resizer/internal/config"
	"imgproxy-resizer/internal/imgproxy"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	prod, err := imgproxy.NewProfile("https://imgproxy.example.com", "736563726574", "73616c7479", false)
	if err != nil {
		t.Fatal(err)
	}
	local, _ := imgproxy.NewProfile("http://localhost:8080", "", "", true)
	s, err := New(map[string]imgproxy.Profile{"prod": prod, "local": local}, Options{
		DefaultProfile: "prod",
		Presets:        []config.Preset{{Name: "Thumbs", Variants: []config.PresetVariant{{Label: "s", Resize: "fit", Width: 320, Height: 180}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func TestIndexAndProfiles(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("index: status %d, type %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`<option value="prod"`, ` selected>`, `data-preset="0">Thumbs<`, `"resizeType":"fit"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("index: missing %q", want)
		}
	}
	resp2, err := http.Get(ts.URL + "/api/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var out struct{ Profiles []ProfileInfo }
	if err := json.NewDecoder(resp2.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Profiles) != 2 || out.Profiles[0].Name != "local" || out.Profiles[1].Name != "prod" {
		t.Fatalf("profiles = %+v", out.Profiles)
	}
	resp3, _ := http.Get(ts.URL + "/static/app.js")
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("static app.js: %d", resp3.StatusCode)
	}
}

func TestGenerateNominal(t *testing.T) {
	ts := newTestServer(t)
	status, out := post(t, ts, `{"profile":"prod","source":"https://imgsource.com/photo.jpg","ext":"jpeg","variants":[
		{"label":"LOW","resizeType":"force","width":820,"height":306,"maxBytes":102400},
		{"label":"HIGH","resizeType":"force","width":1640,"height":616,"enlarge":true,"maxBytes":153600},
		{"label":"BAD","resizeType":"force","width":0,"height":0}
	]}`)
	if status != 200 {
		t.Fatalf("status %d: %v", status, out)
	}
	results := out["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results = %v", results)
	}
	low := results[0].(map[string]any)
	if low["options"] != "rs:force:820:306/mb:102400" {
		t.Errorf("LOW options = %v", low["options"])
	}
	if !strings.HasSuffix(low["url"].(string), "/rs:force:820:306/mb:102400/plain/https://imgsource.com/photo.jpg@jpeg") {
		t.Errorf("LOW url = %v", low["url"])
	}
	high := results[1].(map[string]any)
	if high["options"] != "rs:force:1640:616:1/mb:153600" {
		t.Errorf("HIGH options = %v", high["options"])
	}
	bad := results[2].(map[string]any)
	if bad["error"] == nil || bad["url"] != nil {
		t.Errorf("BAD should carry an error only: %v", bad)
	}
}

func TestGenerateInsecureProfile(t *testing.T) {
	ts := newTestServer(t)
	_, out := post(t, ts, `{"profile":"local","source":"https://a.b/c.png","ext":"png","variants":[{"resizeType":"fit","width":10,"height":10}]}`)
	url := out["results"].([]any)[0].(map[string]any)["url"].(string)
	if url != "http://localhost:8080/insecure/rs:fit:10:10/plain/https://a.b/c.png@png" {
		t.Fatalf("url = %s", url)
	}
}

func TestGenerateErrors(t *testing.T) {
	ts := newTestServer(t)
	cases := map[string]string{
		"unknown profile": `{"profile":"nope","source":"https://a.b/c.png","ext":"jpeg","variants":[{"resizeType":"fit","width":1,"height":1}]}`,
		"no variants":     `{"profile":"prod","source":"https://a.b/c.png","ext":"jpeg","variants":[]}`,
		"bad source":      `{"profile":"prod","source":"c.png","ext":"jpeg","variants":[{"resizeType":"fit","width":1,"height":1}]}`,
		"bad ext":         `{"profile":"prod","source":"https://a.b/c.png","ext":"bmp","variants":[{"resizeType":"fit","width":1,"height":1}]}`,
		"bad json":        `{"profile":`,
		"unknown field":   `{"profile":"prod","source":"https://a.b/c.png","ext":"jpeg","variants":[{"resizeType":"fit","width":1,"height":1,"preset":"x"}]}`,
	}
	for name, body := range cases {
		if status, _ := post(t, ts, body); status != 400 {
			t.Errorf("%s: status %d, want 400", name, status)
		}
	}
}
