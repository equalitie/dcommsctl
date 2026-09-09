package main

import (
	"context"
	"os"
	"path"
	"text/template"
	// "github.com/compose-spec/compose-go/v2/cli"
)

type MastodonConfig struct {
	Domain   string
	PortOut  int
	PortOut2 int
	Version  string
}

func mastodonCreateInstance(ctx context.Context, instancesDir string, config MastodonConfig) (string, error) {
	pathToApp := path.Join("conf_tmpl", "peertube")

	tmpl, err := template.ParseFiles(path.Join(pathToApp, "docker-compose.tmpl.yml"))
	if err != nil {
		return "", err
	}

	fileFlags := os.O_CREATE | os.O_EXCL | os.O_WRONLY

	pathToInst, err := instanceCreate(ctx, instancesDir, "mastodon")

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), fileFlags, 0o644)
	if err != nil {
		return "", err
	}

	if err := tmpl.Execute(composeFile, &config); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles(path.Join(pathToApp, "env"))
	if err != nil {
		return "", err
	}

	envFile, err := os.OpenFile(path.Join(pathToInst, "env"), fileFlags, 0o644)
	if err != nil {
		return "", err
	}

	if err := envTmpl.Execute(envFile, &config); err != nil {
		return "", err
	}

	return pathToInst, nil
}
