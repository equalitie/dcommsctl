package main

import (
	"context"
	"net/http"
	"os"
	"path"
	"text/template"
)

type DawaConfig struct {
	Domain  string
	PortOut int
}

func dawaCreateInstance(ctx context.Context, instancesDir string, config DawaConfig) (string, error) {
	tmpl, err := template.ParseFiles("./conf_tmpl/dawa/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	pathToInst, err := instanceCreate(instancesDir, "dawa")
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

	composeFile, err := os.OpenFile(composePath, fileFlags, 0o644)
	if err != nil {
		return "", err
	}

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

	if err := envTmpl.Execute(envFile, &config); err != nil {
		return "", err
	}

	caddyRouteJSON, err := os.ReadFile("./conf_tmpl/dawa/caddy-route.json")

	endpoint := "http://localhost:2019/config/apps/http/servers/dcomms/routes"
	if err := caddySendRequest(ctx, endpoint, http.MethodPut, caddyRouteJSON); err != nil {
		return "", err
	}

	completed = true

	return pathToInst, nil
}
