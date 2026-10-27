package imgproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Profile is one imgproxy deployment: its public host and signing material.
// When Insecure is set, URLs use the /insecure/ prefix instead of a signature.
type Profile struct {
	Host     string
	Key      []byte
	Salt     []byte
	Insecure bool
}

// NewProfile decodes hex key and salt. Key and salt may be empty only when
// insecure is true.
func NewProfile(host, hexKey, hexSalt string, insecure bool) (Profile, error) {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		return Profile{}, errors.New("host is empty")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		return Profile{}, fmt.Errorf("host %q must start with http:// or https://", host)
	}
	p := Profile{Host: host, Insecure: insecure}
	if insecure {
		return p, nil
	}
	var err error
	if p.Key, err = hex.DecodeString(strings.TrimSpace(hexKey)); err != nil || len(p.Key) == 0 {
		return Profile{}, errors.New("key must be a non-empty hex string")
	}
	if p.Salt, err = hex.DecodeString(strings.TrimSpace(hexSalt)); err != nil || len(p.Salt) == 0 {
		return Profile{}, errors.New("salt must be a non-empty hex string")
	}
	return p, nil
}

// Sign computes the imgproxy signature of a path (which must start with "/"):
// base64url(HMAC-SHA256(key, salt || path)) without padding.
func Sign(key, salt []byte, path string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// EscapeSource prepares a source URL for the "plain" format: it must be an
// absolute http(s) URL, and "@" and "?" are percent-encoded as imgproxy requires.
func EscapeSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("source URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("source must be an absolute http(s) URL")
	}
	s := u.String()
	s = strings.ReplaceAll(s, "@", "%40")
	s = strings.ReplaceAll(s, "?", "%3F")
	return s, nil
}

// BuildPath assembles "/<options>/plain/<source>[@ext]".
func BuildPath(options, source, ext string) (string, error) {
	if !contains(Extensions, ext) {
		return "", fmt.Errorf("format %q: expected one of jpeg, png, webp, avif, gif", ext)
	}
	src, err := EscapeSource(source)
	if err != nil {
		return "", err
	}
	path := "/" + strings.Trim(options, "/") + "/plain/" + src
	if ext != "" {
		path += "@" + ext
	}
	return path, nil
}

// URL returns the full imgproxy URL for the given path on this profile.
func (p Profile) URL(path string) string {
	if p.Insecure {
		return p.Host + "/insecure" + path
	}
	return p.Host + "/" + Sign(p.Key, p.Salt, path) + path
}

// BuildURL renders a variant against a source image on this profile.
func (p Profile) BuildURL(v Variant, source, ext string) (string, error) {
	opts, err := v.Options()
	if err != nil {
		return "", err
	}
	path, err := BuildPath(opts, source, ext)
	if err != nil {
		return "", err
	}
	return p.URL(path), nil
}
