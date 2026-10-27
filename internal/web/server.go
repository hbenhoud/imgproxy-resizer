// Package web serves the single-page UI and the JSON API that signs imgproxy URLs.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"imgproxy-resizer/internal/config"
	"imgproxy-resizer/internal/imgproxy"
)

//go:embed templates/index.html
var templates embed.FS

//go:embed static
var static embed.FS

const maxBodyBytes = 1 << 20

type Server struct {
	profiles       map[string]imgproxy.Profile
	names          []string
	defaultProfile string
	presets        []config.Preset
	tmpl           *template.Template
	mux            *http.ServeMux
}

// Options configures the server beyond the profiles themselves.
type Options struct {
	// DefaultProfile is preselected in the UI; falls back to the first name.
	DefaultProfile string
	// Presets are the one-click variant sets offered by the UI.
	Presets []config.Preset
}

type ProfileInfo struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Insecure bool   `json:"insecure"`
}

type GenerateRequest struct {
	Profile  string             `json:"profile"`
	Source   string             `json:"source"`
	Ext      string             `json:"ext"`
	Variants []imgproxy.Variant `json:"variants"`
}

type Result struct {
	Label   string `json:"label"`
	Options string `json:"options,omitempty"`
	Path    string `json:"path,omitempty"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
}

type GenerateResponse struct {
	Profile string   `json:"profile"`
	Host    string   `json:"host"`
	Results []Result `json:"results"`
}

func New(profiles map[string]imgproxy.Profile, opts Options) (*Server, error) {
	if len(profiles) == 0 {
		return nil, errors.New("no usable profile")
	}
	tmpl, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		return nil, err
	}
	s := &Server{profiles: profiles, names: config.Names(profiles), presets: opts.Presets, tmpl: tmpl, mux: http.NewServeMux()}
	if _, ok := profiles[opts.DefaultProfile]; ok {
		s.defaultProfile = opts.DefaultProfile
	} else {
		s.defaultProfile = s.names[0]
	}
	if s.presets == nil {
		s.presets = []config.Preset{}
	}
	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}
	s.mux.HandleFunc("GET /{$}", s.index)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))
	s.mux.HandleFunc("GET /api/profiles", s.apiProfiles)
	s.mux.HandleFunc("POST /api/generate", s.apiGenerate)
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) profileInfos() []ProfileInfo {
	out := make([]ProfileInfo, 0, len(s.names))
	for _, n := range s.names {
		p := s.profiles[n]
		out = append(out, ProfileInfo{Name: n, Host: p.Host, Insecure: p.Insecure})
	}
	return out
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	presetsJSON, err := json.Marshal(s.presets)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		Profiles    []ProfileInfo
		Default     string
		Presets     []config.Preset
		PresetsJSON template.JS
		ResizeTypes []string
		Extensions  []string
	}{s.profileInfos(), s.defaultProfile, s.presets, template.JS(presetsJSON), imgproxy.ResizeTypes, imgproxy.Extensions}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, data); err != nil {
		log.Printf("render index: %v", err)
	}
}

func (s *Server) apiProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"profiles": s.profileInfos()})
}

func (s *Server) apiGenerate(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}
	p, ok := s.profiles[req.Profile]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown profile %q", req.Profile))
		return
	}
	if len(req.Variants) == 0 {
		writeError(w, http.StatusBadRequest, "at least one variant is required")
		return
	}
	// Validate the shared inputs once so the error is not repeated per row.
	if _, err := imgproxy.BuildPath("rs:fit:1:1", req.Source, req.Ext); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := GenerateResponse{Profile: req.Profile, Host: p.Host, Results: make([]Result, 0, len(req.Variants))}
	for i, v := range req.Variants {
		res := Result{Label: v.Label}
		if res.Label == "" {
			res.Label = fmt.Sprintf("#%d", i+1)
		}
		opts, err := v.Options()
		if err != nil {
			res.Error = err.Error()
			resp.Results = append(resp.Results, res)
			continue
		}
		path, err := imgproxy.BuildPath(opts, req.Source, req.Ext)
		if err != nil {
			res.Error = err.Error()
			resp.Results = append(resp.Results, res)
			continue
		}
		res.Options, res.Path, res.URL = opts, path, p.URL(path)
		resp.Results = append(resp.Results, res)
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
