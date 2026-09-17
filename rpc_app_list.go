package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path"
)

type ApplicationInfo struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	RedirectURL string `json:"redirect_uri"`
}

type ApplicationListResult struct {
	Applications []ApplicationInfo `json:"applications"`
}

func rpcApplicationsList(ctx context.Context) (ApplicationListResult, error) {
	apps := []string{
		"peertube",
		"mastodon",
		"dawa",
	}

	r := ApplicationListResult{}

	for _, p := range apps {
		configPath := path.Join("conf_tmpl", p, "config.json")

		data, err := os.ReadFile(configPath)
		if err != nil {
			log.Printf("apps/list: skipping %s: %s", p, err)
			continue
		}

		app := ApplicationInfo{}

		if err := json.Unmarshal(data, &app); err != nil {
			log.Printf("apps/list: skipping %s: %s", p, err)
			continue
		}

		if app.Id == "" || app.Name == "" {
			log.Printf("apps/list: skipping %s: %s has no id or no name", p, configPath)
			continue
		}

		r.Applications = append(r.Applications, app)
	}

	return r, nil
}
