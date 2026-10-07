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
	"path/filepath"
	"strings"
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

func caddyInstancePath(instancesDir string) (string, error) {
	pathToInst := path.Join(instancesDir, "caddy")
	if _, err := os.Stat(pathToInst); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", err
	}

	return pathToInst, nil
}

func caddySendRequest(ctx context.Context, endpoint, method, etag string, payloadJSON []byte) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		endpoint,
		bytes.NewReader(payloadJSON),
	)
	if err != nil {
		return nil, "", err
	}

	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 4 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Caddy's reply body; a non-2xx status is worth seeing here since the caller only
	// gets an error when the request itself fails.
	logJSON("caddy response", body,
		"endpoint", endpoint, "status", resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, "", fmt.Errorf("Caddy returned %s: %s", resp.Status, body)
	}
	return body, resp.Header.Get("ETag"), nil
}

func caddyAddRoute(ctx context.Context, instanceName, hostname, upstreamAddress string) error {
	// FIXME: change server name
	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"

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

	if _, _, err := caddySendRequest(ctx, endpoint, http.MethodPost, "", payloadJSON); err != nil {
		return err
	}

	return nil
}

func caddyDropRoute(ctx context.Context, instanceName string) error {
	// FIXME: untested, change payload type
	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"

	payload := CaddyRoute{
		ID: instanceName,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, _, err := caddySendRequest(ctx, endpoint, http.MethodDelete, "", payloadJSON); err != nil {
		return err
	}

	return nil
}

func caddyCreateInstance(instancesDir string, config CaddyConfig) (string, error) {
	if config.Domain == "" {
		config.Domain = "localhost"
	}
	if config.Port == 0 {
		config.Port = 8000
	}
	pathToInst := path.Join(instancesDir, "caddy")

	if caddyInstancePath, err := caddyInstancePath(instancesDir); caddyInstancePath != "" {
		return pathToInst, nil
	} else if err != nil {
		return "", err
	}

	tmpl, err := template.ParseFiles("./conf_tmpl/caddy/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	dirsToCreate := []string{
		"jobs",
		"www",
	}

	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(path.Join(pathToInst, dir), 0o700); err != nil {
			return "", fmt.Errorf("create dir %s for instance: %s", dir, err)
		}
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

func caddyExportConfig(ctx context.Context, instancesDir string) error {
	caddyInstanceDir, _ := caddyInstancePath(instancesDir)

	if caddyInstanceDir == "" {
		return fmt.Errorf("Caddy was not installed, something is wrong with your setup, Caddy should be installed by default")
	}

	destination := path.Join(caddyInstanceDir, "config.json")
	endpoint := "http://localhost:2019/config/"
	data, _, err := caddySendRequest(ctx, endpoint, http.MethodGet, "", nil)
	if err != nil {
		return err
	}
	// Preserve raw JSON, including unknown module fields and exact numbers.
	f, err := os.CreateTemp(filepath.Dir(destination), ".caddy-config-*.json")
	if err != nil {
		return fmt.Errorf("create temporary config file: %s", err)
	}
	tempName := f.Name()
	defer func() {
		f.Close()
		os.Remove(tempName)
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write Caddy config: %s", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync Caddy config: %s", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close Caddy config: %s", err)
	}

	if err := os.Rename(tempName, destination); err != nil {
		return fmt.Errorf("replace Caddy config file: %s", err)
	}

	return nil
}

func caddyEnsureTLS(
	ctx context.Context,
	hostname, admin string,
) error {
	admin = strings.TrimRight(admin, "/")

	for attempt := 0; attempt < 5; attempt++ {
		body, etag, err := caddySendRequest(
			ctx, admin+"/config/apps",
			http.MethodGet, "", nil,
		)
		if err != nil {
			return err
		}
		if etag == "" {
			return fmt.Errorf("Caddy response has no ETag")
		}

		var apps struct {
			TLS map[string]any `json:"tls"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if err := decoder.Decode(&apps); err != nil {
			return err
		}

		tls := apps.TLS
		if tls == nil {
			tls = make(map[string]any)
		}
		before, err := json.Marshal(tls)
		if err != nil {
			return err
		}

		certificates, err := ensureObject(tls, "certificates")
		if err != nil {
			return err
		}

		var names []string
		if err := decodeValue(certificates["automate"], &names); err != nil {
			return fmt.Errorf("certificates.automate: %w", err)
		}

		seen := make(map[string]bool)
		unique := make([]string, 0, len(names)+1)
		for _, name := range append(names, hostname) {
			if !seen[name] {
				seen[name] = true
				unique = append(unique, name)
			}
		}
		certificates["automate"] = unique

		automation, err := ensureObject(tls, "automation")
		if err != nil {
			return err
		}

		var policies []map[string]any
		if err := decodeValue(automation["policies"], &policies); err != nil {
			return fmt.Errorf("automation.policies: %w", err)
		}

		covered := false
		for _, policy := range policies {
			var subjects []string
			if err := decodeValue(policy["subjects"], &subjects); err != nil {
				return fmt.Errorf("policy subjects: %w", err)
			}
			for _, subject := range subjects {
				if subject == hostname {
					covered = true
				}
			}
		}

		if !covered {
			policy := map[string]any{
				"subjects": []string{hostname},
				"issuers": []map[string]string{
					{"module": "internal"},
				},
			}
			policies = append([]map[string]any{policy}, policies...)
		}
		automation["policies"] = policies

		after, err := json.Marshal(tls)
		if err != nil {
			return err
		}
		if bytes.Equal(before, after) {
			return nil
		}

		body, _, err = caddySendRequest(
			ctx, admin+"/config/apps/tls",
			http.MethodPost, etag, after,
		)
		if err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("Caddy config kept changing; retry later")
}

func ensureObject(parent map[string]any, key string) (map[string]any, error) {
	if parent[key] == nil {
		parent[key] = make(map[string]any)
	}
	obj, ok := parent[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	return obj, nil
}

func decodeValue(value any, target any) error {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}
