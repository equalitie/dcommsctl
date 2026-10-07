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

type InstanceInstallArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
}

type InstanceInstallResult struct {
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

func (app *GlobalState) rpcInstanceInstall(ctx context.Context, args InstanceInstallArgs) (InstanceInstallResult, error) {
	applicationID := args.AppName
	jobType := "install"
	clientID, _ := randomID()
	clientSecret, _ := randomID()

	switch applicationID {

	case "caddy":
		{
			config := CaddyConfig{
				Domain: args.DomainName,
				Port:   8000,
			}
			instanceDir, err := caddyCreateInstance(app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			return InstanceInstallResult{
				JobID:        "",
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil

		}
	case "mastodon":
		{
			config := MastodonConfig{
				SSOIssuerURI:  app.systemCfg.SSODomain,
				Domain:        args.DomainName,
				PortWebUI:     portRandom(), // 3001,
				PortStreaming: portRandom(), // 4000,
				Version:       "v4.5",
			}

			instanceDir, err := mastodonCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			pass := "test_pass"
			cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, "mastodon", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"VERSION=" + config.Version,
			}

			jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			return InstanceInstallResult{
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
			adminPass := "test_pass"
			config := PeertubeConfig{
				SSOIssuerURI: app.systemCfg.SSODomain,
				Domain:       args.DomainName,
				PortRTMP:     portRandom(), // 1935,
				PortWebUI:    portRandom(), // 9090,
				Db: DbConfig{
					Password: "",
				},
				Admin: AdminConfig{
					Password: adminPass,
				},
			}

			instanceDir, err := peertubeCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, "peertube", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
				"OAUTH_CLIENT_ID=" + clientID,
				"OAUTH_CLIENT_SECRET=" + clientSecret,
			}

			jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			return InstanceInstallResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AdminLogin:   "root",
				AdminPass:    adminPass,
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
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, "dawa", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
			}

			jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			// FIXME: update this in case of failure
			app.systemCfg.SSODomain = args.DomainName

			return InstanceInstallResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil
		}
	}
	return InstanceInstallResult{}, fmt.Errorf("This application is not supported")
}
