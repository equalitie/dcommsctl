package api

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"eq/dcommsctl/internal/application"
)

type BackupsListArgs struct {
	AppName string `json:"filter_by_app"`
	// NOTE: implement later
	// Version string `json:"filter_by_version"`
}

type BackupsListResult struct {
	Backups []string `json:"backups"`
}

// each backup is stored in backupsDir/appName/instanceName/backupID
func getBackups(backupsDir, appName string) ([]string, error) {
	r := []string{}

	instances, err := os.ReadDir(path.Join(backupsDir, appName))
	if err != nil {
		return r, err
	}

	for _, entry := range instances {
		// we only iterate over instances
		if !entry.IsDir() {
			continue
		}

		// now we iterate over folders with backups
		backups, err := os.ReadDir(filepath.Join(backupsDir, appName, entry.Name()))
		if err != nil {
			return r, err
		}
		for _, entry := range backups {
			if !entry.IsDir() {
				continue
			}
			id := entry.Name()

			r = append(r, id)
		}
	}

	return r, nil
}

func (app *GlobalState) BackupsList(ctx context.Context, args BackupsListArgs) (BackupsListResult, error) {
	appName := args.AppName
	filterByApp := appName != ""
	r := BackupsListResult{Backups: []string{}}

	lookIn := application.AppIDs

	if filterByApp {
		lookIn = []string{appName}
	}

	for _, appName := range lookIn {
		l, err := getBackups(app.SystemCfg.BackupsDir, appName)
		if err != nil {
			return r, nil
		}

		r.Backups = append(r.Backups, l...)
	}

	return r, nil
}
