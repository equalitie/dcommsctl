package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
)

type DawaConfig struct {
	Domain       string
	PortOut      int
	NginxDocRoot string
}

func dawaIsInstalled(instancesDir string) (bool, error) {
	pathToInst := path.Join(instancesDir, "dawa")
	if _, err := os.Stat(pathToInst); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func dawaCreateInstance(ctx context.Context, instancesDir string, config DawaConfig) (string, error) {
	// FIXME: this is a temporary measure, cause we ideally want to have multiple instances of dawa
	// or maybe we need another mechanism to upgrade dawa
	pathToApp := path.Join("conf_tmpl", "dawa")
	pathToInst := path.Join(instancesDir, "dawa")
	if installed, _ := dawaIsInstalled(instancesDir); installed {
		return pathToInst, nil
	}

	completed := false
	defer func() {
		if !completed {
			os.RemoveAll(pathToInst)
		}
	}()

	dirsToCreate := []string{
		"jobs",
	}

	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(path.Join(pathToInst, dir), 0o700); err != nil {
			return "", fmt.Errorf("create dir %s for instance: %s", dir, err)
		}
	}

	if err := executeTemplate(
		path.Join(pathToApp, "docker-compose.tmpl.yml"),
		path.Join(pathToInst, "docker-compose.yml"),
		&config,
	); err != nil {
		return "", err
	}

	if err := executeTemplate(
		path.Join(pathToApp, "env.tmpl"),
		path.Join(pathToInst, "env"),
		&config,
	); err != nil {
		return "", err
	}

	caddyRouteFilePath := path.Join(pathToInst, "caddy-route.json")
	if err := executeTemplate(
		path.Join(pathToApp, "caddy-route.tmpl.json"),
		caddyRouteFilePath,
		&config,
	); err != nil {
		return "", err
	}

	caddyRouteJSON, err := os.ReadFile(caddyRouteFilePath)
	if err != nil {
		return "", err
	}

	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"
	if _, err := caddySendRequest(ctx, endpoint, http.MethodPost, caddyRouteJSON); err != nil {
		return "", err
	}

	if err := caddyExportConfig(ctx, instancesDir); err != nil {
		return "", err
	}

	completed = true

	return pathToInst, nil
}
