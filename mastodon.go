package main

import (
	"context"
	"fmt"
	"path"
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

	pathToInst, err := instanceCreate(instancesDir, "mastodon")
	if err != nil {
		return "", err
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

	if err := executeTemplate(
		path.Join(pathToApp, "caddy-route.tmpl.json"),
		path.Join(pathToInst, "caddy-route.json"),
		&config,
	); err != nil {
		return "", err
	}

	return pathToInst, nil
}
