package proxy

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
	"time"

	"eq/dcommsctl/internal/log"
)

// caddy is a proxy for other services so it's basically a single instance service

type ProxyConfig struct {
	Domain    string
	HTTPPort  int
	HTTPSPort int
	// Serve everything on :80 without TLS; ACME cannot work behind the 8000/3443 mapping.
	PlainHTTP bool
}

type CaddyRoute struct {
	ID string `json:"@id"`
}

func CaddySendRequest(ctx context.Context, endpoint, method, etag string, payloadJSON []byte) ([]byte, string, error) {
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
	log.LogJSON("caddy response", body,
		"endpoint", endpoint, "status", resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, "", fmt.Errorf("Caddy returned %s: %s", resp.Status, body)
	}
	return body, resp.Header.Get("ETag"), nil
}

func DropRoute(ctx context.Context, instanceID string) error {
	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"

	payload := CaddyRoute{
		ID: instanceID,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, _, err := CaddySendRequest(ctx, endpoint, http.MethodDelete, "", payloadJSON); err != nil {
		return err
	}

	return nil
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

func ExportConfig(ctx context.Context, instancesDir string) error {
	caddyInstanceDir, _ := caddyInstancePath(instancesDir)

	if caddyInstanceDir == "" {
		return fmt.Errorf("Caddy was not installed, something is wrong with your setup, Caddy should be installed by default")
	}

	destination := path.Join(caddyInstanceDir, "config.json")
	endpoint := "http://localhost:2019/config/"
	data, _, err := CaddySendRequest(ctx, endpoint, http.MethodGet, "", nil)
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
