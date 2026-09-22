package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
)

type InstallInstanceArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
}

type InstallInstanceResult struct {
	JobID        string `json:"job_id"`
	ID           string `json:"id"`
	AdminLogin   string `json:"admin_login"`
	AdminPass    string `json:"admin_pass"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func randomID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (app *GlobalState) rpcInstanceInstall(ctx context.Context, args InstallInstanceArgs) (InstallInstanceResult, error) {
	applicationID := args.AppName
	jobType := "install"
	jobID := jobID(jobType)

	clientID, _ := randomID()
	clientSecret, _ := randomID()

	switch applicationID {

	case "caddy":
		{
			config := CaddyConfig{
				Domain: args.DomainName,
				Port:   8000,
			}
			instanceDir, err := caddyCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			return InstallInstanceResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil

		}
	case "mastodon":
		{
			config := MastodonConfig{
				Domain:        args.DomainName,
				PortWebUI:     3000,
				PortStreaming: 4000,
				Version:       "v4.5",
			}

			instanceDir, err := mastodonCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, "."+jobType+".lock")

			lockFile := jobLockFile(lockFilePath)

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}
			defer lockFile.Close()

			pass := "test_pass"
			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.AppsDir, "mastodon", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"VERSION=" + config.Version,
			}

			if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AdminLogin:   "root",
				AdminPass:    pass,
			}, nil
		}
	case "peertube":
		{
			config := PeertubeConfig{
				Domain:   args.DomainName,
				PortOut:  1935,
				PortOut2: 9090,
				Db: DbConfig{
					Password: "",
				},
			}

			instanceDir, err := peertubeCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, "."+jobType+".lock")

			lockFile := jobLockFile(lockFilePath)

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}
			defer lockFile.Close()

			pass := "test_pass"
			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.AppsDir, "peertube", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
				"PEERTUBE_ADMIN_PASS=" + pass,
				"DWEB_DOMAIN=" + args.DomainName,
				"OAUTH_CLIENT_ID=" + clientID,
				"OAUTH_CLIENT_SECRET=" + clientSecret,
			}

			if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AdminLogin:   "root",
				AdminPass:    pass,
			}, nil
		}
	case "dawa":
		{
			config := DawaConfig{
				Domain:  args.DomainName,
				PortOut: 8081,
			}

			instanceDir, err := dawaCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := "." + jobType + ".lock"

			lockFile := jobLockFile(path.Join(instanceDir, lockFilePath))

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.AppsDir, "dawa", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"LOCK_FILE=" + lockFilePath,
				"APP_DIR=" + instanceDir,
			}

			if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil
		}
	}
	return InstallInstanceResult{}, fmt.Errorf("This application is not supported")
}
