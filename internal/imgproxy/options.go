// Package imgproxy builds and signs imgproxy URLs from free-form processing
// options (no presets), following the URL format documented at
// https://docs.imgproxy.net/usage/processing.
package imgproxy

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Resize types supported by imgproxy's `rs` option.
var ResizeTypes = []string{"force", "fit", "fill", "fill-down", "auto"}

// Output formats accepted as `@ext` suffix. Empty keeps the source format.
var Extensions = []string{"jpeg", "png", "webp", "avif", "gif", ""}

// extraSegment matches one raw option such as `bl:0.5`, `dpr:2` or `g:ce`.
var extraSegment = regexp.MustCompile(`^[a-z][a-z_]*:[A-Za-z0-9_.:%-]*$`)

// Variant describes one output to produce from the source image.
// Zero values mean "option not set".
type Variant struct {
	Label      string  `json:"label"`
	ResizeType string  `json:"resizeType"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Enlarge    bool    `json:"enlarge"`
	Extend     bool    `json:"extend"`
	Quality    int     `json:"quality"`
	MaxBytes   int     `json:"maxBytes"`
	Sharpen    float64 `json:"sharpen"`
	Extra      string  `json:"extra"`
}

// Validate checks the variant fields against imgproxy constraints.
func (v Variant) Validate() error {
	if !contains(ResizeTypes, v.ResizeType) {
		return fmt.Errorf("resize type %q: expected one of %s", v.ResizeType, strings.Join(ResizeTypes, ", "))
	}
	if v.Width < 0 || v.Height < 0 {
		return errors.New("width and height must be >= 0")
	}
	if v.Width == 0 && v.Height == 0 {
		return errors.New("width or height must be > 0")
	}
	if v.Quality < 0 || v.Quality > 100 {
		return errors.New("quality must be between 1 and 100")
	}
	if v.MaxBytes < 0 {
		return errors.New("max bytes must be >= 0")
	}
	if v.Sharpen < 0 {
		return errors.New("sharpen must be >= 0")
	}
	for _, seg := range splitExtra(v.Extra) {
		if !extraSegment.MatchString(seg) {
			return fmt.Errorf("extra option %q: expected name:value segments separated by /", seg)
		}
	}
	return nil
}

// Options renders the processing options segment, e.g.
// "rs:force:820:306:1/q:85/mb:102400/sh:0.4/bl:0.5" (no leading slash).
func (v Variant) Options() (string, error) {
	if err := v.Validate(); err != nil {
		return "", err
	}
	rs := fmt.Sprintf("rs:%s:%d:%d", v.ResizeType, v.Width, v.Height)
	switch {
	case v.Extend:
		rs += fmt.Sprintf(":%s:1", boolFlag(v.Enlarge))
	case v.Enlarge:
		rs += ":1"
	}
	parts := []string{rs}
	if v.Quality > 0 {
		parts = append(parts, "q:"+strconv.Itoa(v.Quality))
	}
	if v.MaxBytes > 0 {
		parts = append(parts, "mb:"+strconv.Itoa(v.MaxBytes))
	}
	if v.Sharpen > 0 {
		parts = append(parts, "sh:"+strconv.FormatFloat(v.Sharpen, 'f', -1, 64))
	}
	parts = append(parts, splitExtra(v.Extra)...)
	return strings.Join(parts, "/"), nil
}

func splitExtra(extra string) []string {
	var out []string
	for _, seg := range strings.Split(extra, "/") {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
