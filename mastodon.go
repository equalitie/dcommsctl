package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"text/template"
	// "github.com/compose-spec/compose-go/v2/cli"
)

type MastodonConfig struct {
	InstanceGenericConfig
	PortStreaming int
	PortWebUI     int
}

func mastodonCreateInstance(ctx context.Context, instancesDir string, config MastodonConfig) (string, error) {
	if config.Domain == "" {
		return "", fmt.Errorf("Mastodon instance requires you to specify a domain")
	}
	// FIXME: we need to figure out the strategy for that. we should allow to bind the ports that are already bound if we want to swap instances but we also need a way to update those ports if we used wrong ones by mistake
	if !portIsBindable(config.PortWebUI) {
		return "", fmt.Errorf("Mastodon: port is already bound: %d", config.PortWebUI)
	}
	if !portIsBindable(config.PortStreaming) {
		return "", fmt.Errorf("Mastodon: port is already bound: %d", config.PortStreaming)
	}
	pathToApp := path.Join("conf_tmpl", "mastodon")

	tmpl, err := template.ParseFiles(path.Join(pathToApp, "docker-compose.tmpl.yml"))
	if err != nil {
		return "", err
	}

	fileFlags := os.O_CREATE | os.O_EXCL | os.O_WRONLY

	pathToInst, err := instanceCreate(instancesDir, "mastodon")
	if err != nil {
		return "", err
	}

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), fileFlags, 0o644)
	if err != nil {
		return "", err
	}

	if err := tmpl.Execute(composeFile, &config); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles(path.Join(pathToApp, "env.tmpl"))
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
