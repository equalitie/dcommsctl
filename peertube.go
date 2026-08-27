package main

import (
	"context"
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
	Domain   string
	PortOut  int
	PortOut2 int
	Db       DbConfig
}

func peertubeCreateInstance(ctx context.Context, instancesDir string) (string, error) {
	tmpl, err := template.ParseFiles("./conf_tmpl/peertube/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	pathToInst, err := instanceCreate(ctx, instancesDir, "peertube")

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	data := PeertubeConfig{
		Domain:   "peertube.",
		PortOut:  1935,
		PortOut2: 9090,
		Db: DbConfig{
			Password: "",
		},
	}

	if err := tmpl.Execute(composeFile, &data); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles("./conf_tmpl/peertube/env")
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
