package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		data, err := os.ReadFile(path.Join("conf_tmpl", p, "config.json"))
		if err != nil {
			fmt.Println(err)
		}
		app := ApplicationInfo{}

		if err := json.Unmarshal(data, &app); err != nil {
			fmt.Println(err)
		}

		r.Applications = append(r.Applications, app)
	}

	return r, nil
}
