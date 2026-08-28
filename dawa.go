package main

import (
	"context"
	"os"
	"path"
	"text/template"
)

type DawaConfig struct {
	Domain  string
	PortOut int
}

func dawaCreateInstance(ctx context.Context, instancesDir string) (string, error) {
	tmpl, err := template.ParseFiles("./conf_tmpl/dawa/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	pathToInst, err := instanceCreate(ctx, instancesDir, "dawa")

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	data := PeertubeConfig{
		Domain:  "dawa.",
		PortOut: 8081,
	}

	if err := tmpl.Execute(composeFile, &data); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles("./conf_tmpl/dawa/env")
	if err != nil {
		return "", err
	}

	envFile, err := os.OpenFile(path.Join(pathToInst, "env"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	if err := envTmpl.Execute(envFile, &data); err != nil {
		return "", err
	}

	return pathToInst, nil
}
