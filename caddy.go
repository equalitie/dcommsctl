package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"text/template"
)

// caddy is a proxy for other services so it's basically a single instance service

type CaddyConfig struct {
	Domain string
	Port   int
}

func caddyIsInstalled(instancesDir string) (bool, error) {
	pathToInst := path.Join(instancesDir, "caddy")
	if _, err := os.Stat(pathToInst); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func caddyCreateInstance(ctx context.Context, instancesDir string, config CaddyConfig) (string, error) {
	pathToInst := path.Join(instancesDir, "caddy")

	if alreadyInstalled, err := caddyIsInstalled(instancesDir); alreadyInstalled {
		return pathToInst, nil
	} else if err != nil {
		return "", err
	}

	tmpl, err := template.ParseFiles("./conf_tmpl/caddy/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	folderToCreate := path.Join(pathToInst, "jobs")

	if err := os.MkdirAll(folderToCreate, 0o700); err != nil {
		return "", fmt.Errorf("create instance dir: %s", err)
	}

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	if err := tmpl.Execute(composeFile, &config); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles("./conf_tmpl/caddy/Caddyfile.tmpl")
	if err != nil {
		return "", err
	}

	envFile, err := os.OpenFile(path.Join(pathToInst, "Caddyfile"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	if err := envTmpl.Execute(envFile, &config); err != nil {
		return "", err
	}

	return pathToInst, nil
}
