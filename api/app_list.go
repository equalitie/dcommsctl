package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"eq/dcommsctl/internal/application"
	"eq/dcommsctl/internal/log"
)

// One published version. Hand-maintained in each config.json; only Version is required.
type Release struct {
	Version    string `json:"version"`
	ReleasedAt string `json:"released_at,omitempty"`
	Changelog  string `json:"changelog,omitempty"`
}

// Summary shape returned by apps/list.
type ApplicationInfo struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	RedirectURL string `json:"redirect_uri"`
	Description string `json:"description,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// Full shape returned by apps/get. Embeds the summary so the two cannot drift.
type ApplicationDetail struct {
	ApplicationInfo
	LongDescription string    `json:"long_description,omitempty"`
	UpstreamURL     string    `json:"upstream_url,omitempty"`
	DefinitionURL   string    `json:"definition_url,omitempty"`
	Releases        []Release `json:"releases"`
}

type ApplicationListResult struct {
	Applications []ApplicationInfo `json:"applications"`
}

// Reads one application definition. Callers must check the id against appIDs first.
func loadAppConfig(id string) (ApplicationDetail, error) {
	configPath := path.Join("conf_tmpl", id, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ApplicationDetail{}, err
	}

	app := ApplicationDetail{}

	if err := json.Unmarshal(data, &app); err != nil {
		return ApplicationDetail{}, fmt.Errorf("%s: %s", configPath, err)
	}

	if app.Id == "" || app.Name == "" {
		return ApplicationDetail{}, fmt.Errorf("%s: no id or no name", configPath)
	}

	// Serialize an absent history as [] rather than null.
	if app.Releases == nil {
		app.Releases = []Release{}
	}

	return app, nil
}

func ApplicationsList(ctx context.Context) (ApplicationListResult, error) {
	r := ApplicationListResult{}

	// Skip definitions that fail to load; one bad file must not fail the whole call.
	for _, id := range application.AppIDs {
		app, err := loadAppConfig(id)
		if err != nil {
			log.LogInfo("app/list skipping", "app", id, "err", err)
			continue
		}

		r.Applications = append(r.Applications, app.ApplicationInfo)
	}

	return r, nil
}
