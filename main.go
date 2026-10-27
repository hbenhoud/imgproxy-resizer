// imgproxy-resizer serves a local web UI that signs imgproxy URLs built from
// free-form processing options, to test display creative resizing without
// touching the deployed presets.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"imgproxy-resizer/internal/config"
	"imgproxy-resizer/internal/web"
)

func main() {
	cfgPath := flag.String("config", "config.yml", "path to the YAML config (see config.example.yml)")
	addr := flag.String("addr", "", "listen address, overrides config `listen` (default 127.0.0.1:8099)")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	profiles, skipped := cfg.Build()
	for name, err := range skipped {
		log.Printf("WARN profile %q disabled: %v", name, err)
	}
	if len(profiles) == 0 {
		fmt.Fprintln(os.Stderr, "no usable profile: set the key/salt env vars or edit", *cfgPath)
		os.Exit(1)
	}
	for _, n := range config.Names(profiles) {
		log.Printf("profile %q -> %s (insecure=%t)", n, profiles[n].Host, profiles[n].Insecure)
	}

	srv, err := web.New(profiles, web.Options{DefaultProfile: cfg.DefaultProfile, Presets: cfg.Presets})
	if err != nil {
		log.Fatalf("init server: %v", err)
	}
	listen := cfg.Listen
	if *addr != "" {
		listen = *addr
	}
	log.Printf("listening on http://%s", listen)
	httpSrv := &http.Server{Addr: listen, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
