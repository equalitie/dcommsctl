package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type InstancesListArgs struct{}

type InstanceInfo struct {
	ID      string `json:"id"`
	App     string `json:"app"`
	Domain  string `json:"domain"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type InstancesListResult struct {
	Instances []InstanceInfo `json:"instances"`
}

// domainEnvKeys maps an app name to the env var its template writes the
// instance's domain under. Every app template uses a different name, so
// there is no way to read this generically; caddy has none (its domain
// lives in the Caddyfile, not env), so it is left unmapped.
var domainEnvKeys = map[string]string{
	"peertube": "PEERTUBE_WEBSERVER_HOSTNAME",
	"dawa":     "SITE_DOMAIN",
	"mastodon": "LOCAL_DOMAIN",
}

// instanceApp recovers the app an instance was created from out of its
// directory name, which instanceCreate always writes as "<app>-<animal>"
// (or, for caddy, just "caddy").
func instanceApp(instanceID string) string {
	if idx := strings.LastIndex(instanceID, "-"); idx >= 0 {
		return instanceID[:idx]
	}
	return instanceID
}

// instanceDomain best-effort reads the domain out of the instance's env
// file, since the backend does not otherwise track it per instance.
func instanceDomain(instanceDir, appName string) string {
	key, ok := domainEnvKeys[appName]
	if !ok {
		return ""
	}

	env, err := godotenv.Read(path.Join(instanceDir, "env"))
	if err != nil {
		return ""
	}

	return env[key]
}

// instanceVersion reads the version the app ships in its catalog config.
// There is no per-instance upgrade tracking yet (inst/upgrade is a stub),
// so this is the catalog version, not necessarily what is actually running.
func instanceVersion(appName string) string {
	data, err := os.ReadFile(path.Join("conf_tmpl", appName, "config.json"))
	if err != nil {
		return ""
	}

	info := ApplicationInfo{}
	if err := json.Unmarshal(data, &info); err != nil {
		return ""
	}

	return info.Version
}

// instanceStatus asks docker compose whether the instance has any running
// containers. Any failure (docker unreachable, no compose file, ...) is
// reported as an empty/unknown status rather than failing the whole list.
func instanceStatus(ctx context.Context, instanceDir string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--status", "running", "-q")
	cmd.Dir = instanceDir

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	if strings.TrimSpace(string(out)) == "" {
		return "stopped"
	}
	return "running"
}

func (app *GlobalState) rpcInstancesList(ctx context.Context, args InstancesListArgs) (InstancesListResult, error) {
	r := InstancesListResult{Instances: []InstanceInfo{}}

	entries, err := os.ReadDir(app.systemCfg.InstancesDir)
	if err != nil {
		return r, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		appName := instanceApp(id)
		instanceDir := path.Join(app.systemCfg.InstancesDir, id)

		r.Instances = append(r.Instances, InstanceInfo{
			ID:      id,
			App:     appName,
			Domain:  instanceDomain(instanceDir, appName),
			Version: instanceVersion(appName),
			Status:  instanceStatus(ctx, instanceDir),
		})
	}

	return r, nil
}
