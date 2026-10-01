package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"text/template"
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
	pathToInst := path.Join(instancesDir, "dawa")
	if installed, _ := dawaIsInstalled(instancesDir); installed {
		return pathToInst, nil
	}

	tmpl, err := template.ParseFiles("./conf_tmpl/dawa/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	composePath := path.Join(pathToInst, "docker-compose.yml")
	envPath := path.Join(pathToInst, "env")
	fileFlags := os.O_CREATE | os.O_EXCL | os.O_WRONLY

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

	composeFile, err := os.OpenFile(composePath, fileFlags, 0o644)
	if err != nil {
		return "", err
	}
	defer composeFile.Close()

	if err := tmpl.Execute(composeFile, &config); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles("./conf_tmpl/dawa/env")
	if err != nil {
		return "", err
	}

	envFile, err := os.OpenFile(envPath, fileFlags, 0o644)
	if err != nil {
		return "", err
	}
	defer envFile.Close()

	if err := envTmpl.Execute(envFile, &config); err != nil {
		return "", err
	}

	routeTmpl, err := template.ParseFiles("./conf_tmpl/dawa/caddy-route.json")
	if err != nil {
		return "", err
	}

	caddyRouteFilePath := path.Join(pathToInst, "caddy-route.json")

	caddyFile, err := os.OpenFile(caddyRouteFilePath, fileFlags, 0o644)
	if err != nil {
		return "", err
	}
	defer caddyFile.Close()

	if err := routeTmpl.Execute(caddyFile, &config); err != nil {
		return "", err
	}
	caddyRouteJSON, err := os.ReadFile(caddyRouteFilePath)

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
