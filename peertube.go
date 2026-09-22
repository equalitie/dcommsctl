package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"text/template"
)

type AdminConfig struct {
	Email string
}

type DbConfig struct {
	Password string
}

type PeertubeConfig struct {
	InstanceGenericConfig
	PortOut  int
	PortOut2 int
	Db       DbConfig
}

func peertubeCreateInstance(ctx context.Context, instancesDir string, config PeertubeConfig) (string, error) {
	pathToApp := path.Join("conf_tmpl", "peertube")
	tmpl, err := template.ParseFiles(path.Join(pathToApp, "docker-compose.tmpl.yml"))
	if err != nil {
		return "", err
	}

	pathToInst, err := instanceCreate(ctx, instancesDir, "peertube")

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
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

	envFile, err := os.OpenFile(path.Join(pathToInst, "env"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	if err := envTmpl.Execute(envFile, &config); err != nil {
		return "", err
	}

	return pathToInst, nil
}

func peertubeEnable(ctx context.Context, instanceName string, config PeertubeConfig) error {
	if err := caddyAddRoute(ctx, instanceName, fmt.Sprintf("%s:%d", config.Domain, config.PortOut), fmt.Sprintf("http://%s-peertube-back:%d", instanceName, config.PortOut)); err != nil {
		return err
	}
	if err := caddyAddRoute(ctx, instanceName, config.Domain, fmt.Sprintf("http://%s-peertube-back:%d", instanceName, config.PortOut)); err != nil {
		return err
	}
	return nil
}

func peertubeDisable(ctx context.Context, instanceName string) error {
	return nil
}
