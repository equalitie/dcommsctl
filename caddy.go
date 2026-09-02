package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"text/template"
	"time"
)

// caddy is a proxy for other services so it's basically a single instance service

type CaddyConfig struct {
	Domain string
	Port   int
}

type upstream struct {
	Dial string `json:"dial"`
}
type reverseProxyHandler struct {
	Handler   string     `json:"handler"`
	Upstreams []upstream `json:"upstreams"`
}

type hostMatcher struct {
	Host []string `json:"host"`
}

type CaddyRoute struct {
	ID       string                `json:"@id"`
	Match    []hostMatcher         `json:"match"`
	Handle   []reverseProxyHandler `json:"handle"`
	Terminal bool                  `json:"terminal"`
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

func caddySendRequest(ctx context.Context, endpoint string, method string, payloadJSON []byte) error {
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		endpoint,
		bytes.NewReader(payloadJSON),
	)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 4 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println(body)

	return nil
}

func caddyAddRoute(ctx context.Context, instanceName, hostname, upstreamAddress string) error {
	endpoint := "localhost:2019/config/apps/http/servers/dcomms/routes"

	payload := CaddyRoute{
		ID: instanceName,
		Match: []hostMatcher{
			{Host: []string{hostname}},
		},
		Handle: []reverseProxyHandler{
			{
				Handler: "reverse_proxy",
				Upstreams: []upstream{
					{Dial: upstreamAddress},
				},
			},
		},
		Terminal: true,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if err := caddySendRequest(ctx, endpoint, http.MethodPost, payloadJSON); err != nil {
		return err
	}

	return nil
}

func caddyDropRoute(ctx context.Context, instanceName string) error {
	// FIXME: untested, change payload type
	endpoint := "localhost:2019/config/apps/http/servers/dcomms/routes"

	payload := CaddyRoute{
		ID: instanceName,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := caddySendRequest(ctx, endpoint, http.MethodDelete, payloadJSON); err != nil {
		return err
	}

	return nil
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
