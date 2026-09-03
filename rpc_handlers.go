package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

type InstallInstanceArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
}

type InstallInstanceResult struct {
	JobID      string `json:"job_id"`
	ID         string `json:"id"`
	AdminLogin string `json:"admin_login"`
	AdminPass  string `json:"admin_pass"`
}

func (app *GlobalState) rpcInstanceInstall(ctx context.Context, args InstallInstanceArgs) (InstallInstanceResult, error) {
	applicationID := args.AppName
	jobType := "install"
	jobID := jobID(jobType)

	switch applicationID {

	case "caddy":
		{
			config := CaddyConfig{
				Domain: args.DomainName,
			}
			instanceDir, err := caddyCreateInstance(ctx, app.systemCfg.InstancesDir, config)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			return InstallInstanceResult{
				JobID: jobID,
				ID:    instanceID,
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
			lockFileName := "." + jobType + ".lock"

			lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}
			defer lockFile.Close()

			pass := "test_pass"
			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.DcommsConfFilesDir, "peertube_install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"LOCK_FILE=" + lockFileName,
				"APP_DIR=" + instanceDir,
				"PEERTUBE_ADMIN_PASS=" + pass,
				"DWEB_DOMAIN=" + args.DomainName,
			}

			if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID:      jobID,
				ID:         instanceID,
				AdminLogin: "root",
				AdminPass:  pass,
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
			lockFileName := "." + jobType + ".lock"

			lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}
			defer lockFile.Close()

			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.ScriptsDir, "dawa_install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"LOCK_FILE=" + lockFileName,
				"APP_DIR=" + instanceDir,
			}

			if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID: jobID,
				ID:    instanceID,
			}, nil
		}
	}
	return InstallInstanceResult{}, nil
}

type RemoveAppArgs struct {
	ID string `json:"id"`
}
type InstanceUninstallResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceUninstall(ctx context.Context, args RemoveAppArgs) (InstanceUninstallResult, error) {
	jobType := "uninstall"

	jobID := jobID(jobType)

	instanceID := args.ID

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFileName := path.Join(instanceDir, "."+jobType+".lock")

	lockFile := jobLockFile(lockFileName)

	if lockFile == nil {
		return InstanceUninstallResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	cmd := exec.CommandContext(ctx, "docker compose down")
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"LOCK_FILE=" + lockFileName,
		"APP_DIR=" + instanceDir,
	}

	if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
		return InstanceUninstallResult{}, err
	}

	return InstanceUninstallResult{
		JobID: jobID,
	}, nil
}

type InstanceStartArgs struct {
	InstanceName string `json:"id"`
}

type InstanceStartResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStart(ctx context.Context, args InstanceStartArgs) (InstanceStartResult, error) {
	jobType := "start"

	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFileName := path.Join(instanceDir, "."+jobType+".lock")

	lockFile := jobLockFile(lockFileName)

	if lockFile == nil {
		return InstanceStartResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	cmdString := fmt.Sprintf("docker compose up -d -p %s", instanceID)

	cmd := exec.CommandContext(ctx, cmdString)
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"LOCK_FILE=" + lockFileName,
		"APP_DIR=" + instanceDir,
	}

	if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
		return InstanceStartResult{}, err
	}

	return InstanceStartResult{
		JobID: jobID,
	}, nil
}

type InstanceStopArgs struct {
	InstanceName string `json:"id"`
}

type InstanceStopResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStop(ctx context.Context, args InstanceStopArgs) (InstanceStopResult, error) {
	jobType := "stop"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFileName := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

	if lockFile == nil {
		return InstanceStopResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	cmd := exec.CommandContext(ctx, "docker compose down")
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"LOCK_FILE=" + lockFileName,
		"APP_DIR=" + instanceDir,
	}

	if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
		return InstanceStopResult{}, err
	}

	return InstanceStopResult{
		JobID: jobID,
	}, nil
}

type InstanceBackupArgs struct {
	InstanceName string `json:"id"`
}

type InstanceBackupResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceBackup(ctx context.Context, args InstanceBackupArgs) (InstanceBackupResult, error) {
	jobType := "backup"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFileName := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

	if lockFile == nil {
		return InstanceBackupResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	backupID := instanceID
	backupRoot := path.Join(app.systemCfg.BackupsDir, backupID)

	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return InstanceBackupResult{}, err
	}

	cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.ScriptsDir, "peertube_backup.sh"))
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
		"LOCK_FILE=" + lockFileName,
		"APP_DIR=" + instanceDir,
	}
	if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
		return InstanceBackupResult{}, err
	}

	return InstanceBackupResult{
		JobID: jobID,
	}, nil
}

type InstanceRestoreArgs struct {
	InstanceName string `json:"id"`
}

type InstanceRestoreResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceRestore(ctx context.Context, args InstanceRestoreArgs) (InstanceRestoreResult, error) {
	jobType := "backup"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFileName := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

	if lockFile == nil {
		return InstanceRestoreResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()
	backupID := instanceID
	backupRoot := path.Join(app.systemCfg.BackupsDir, backupID)

	if _, err := os.Stat(backupRoot); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("Failed to get access to backup dir: %s", err)
	}

	cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.ScriptsDir, "peertube_restore.sh"))
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
		"LOCK_FILE=" + lockFileName,
		"APP_DIR=" + instanceDir,
	}
	if err := app.jobsManager.startExec(ctx, instanceDir, jobID, cmd); err != nil {
		return InstanceRestoreResult{}, err
	}

	return InstanceRestoreResult{
		JobID: jobID,
	}, nil
}
