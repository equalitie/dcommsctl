package main

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
)

type AdminConfig struct {
	Email    string
	Password string
}

type DbConfig struct {
	Password string
}

type PeertubeConfig struct {
	InstanceGenericConfig
	PortRTMP  int
	PortWebUI int
	Db        DbConfig
	Admin     AdminConfig
}

func peertubeCreateInstance(ctx context.Context, instancesDir string, config PeertubeConfig) (string, error) {
	if !portIsBindable(config.PortRTMP) {
		return "", fmt.Errorf("Peertube: port is already bound: %d", config.PortRTMP)
	}
	if !portIsBindable(config.PortWebUI) {
		return "", fmt.Errorf("Peertube: port is already bound: %d", config.PortWebUI)
	}

	pathToApp := path.Join("conf_tmpl", "peertube")
	pathToInst, err := instanceCreate(instancesDir, "peertube")
	if err != nil {
		return "", err
	}

	instanceID := filepath.Base(pathToInst)

	config.InstanceID = instanceID

	if err := executeTemplate(
		path.Join(pathToApp, "docker-compose.tmpl.yml"),
		path.Join(pathToInst, "docker-compose.yml"),
		&config,
	); err != nil {
		return "", err
	}

	if err := executeTemplate(
		path.Join(pathToApp, "env.tmpl"),
		path.Join(pathToInst, ".env"),
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

	if err := caddyExportConfig(ctx, instancesDir); err != nil {
		return "", err
	}

	return pathToInst, nil
}

func peertubeEnable(ctx context.Context, instanceName string, config PeertubeConfig) error {
	if err := caddyAddRoute(ctx, instanceName, fmt.Sprintf("%s:%d", config.Domain, config.PortRTMP), fmt.Sprintf("http://%s-peertube-back:%d", instanceName, config.PortRTMP)); err != nil {
		return err
	}
	if err := caddyAddRoute(ctx, instanceName, config.Domain, fmt.Sprintf("http://%s-peertube-back:%d", instanceName, config.PortRTMP)); err != nil {
		return err
	}
	return nil
}

func peertubeDisable(ctx context.Context, instanceName string) error {
	return nil
}
