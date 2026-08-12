package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

var (
	//go:embed debian_setup.sh
	installScriptContents []byte
	//go:embed debian_check.sh
	checkOsScriptContents []byte
)

// I used this to run backup scripts, this is not needed anymore, but is a good alternative to a naive execCommand calls
func scriptRun(ctx context.Context, scriptPath string, args []string, workDir string, envFile string, stderr io.Writer) (string, error) {
	var err error
	var fileEnv []string

	// TODO: read env vars from envFile, need a simple solution

	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Dir = workDir
	cmd.Env, err = mergeEnvVars(cmd.Environ(), fileEnv)
	if err != nil {
		return "", nil
	}

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		// send SIGTERM for script traps to cleanup
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}

	cmd.WaitDelay = 30 * time.Second

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("exec command: %s %s", scriptPath, err)
	}

	return "", nil
}

func mergeEnvVars(base []string, newValues []string) ([]string, error) {
	// TODO: implement merging of env vars
	return nil, nil
}

func scriptInstall(ctx context.Context, t DeploymentTargetInfo) error {
	choices := []string{}

	if t.PeertubeEnabled {
		choices = append(choices, "peertube")
	}
	if t.MastodonEnabled {
		choices = append(choices, "mastodon")
	}

	if len(choices) == 0 {
		return fmt.Errorf("Install ran without any services selected")
	}

	if err := sshCopyProject(t); err != nil {
		return fmt.Errorf("Failed to copy project files: %s", err)
	}

	cmd := exec.Command("ssh", "-o", "ControlMaster=no", "-o", "ControlPath=none", t.User+"@"+t.Host, "bash -c", string(installScriptContents))
	// FIXME: we need a unified output buffer we're sending back to the client
	// TODO: we need to also stream this, not wait for the whole output and probably leave some markers in output to check for progress
	output, err := cmd.CombinedOutput()
	fmt.Println("--- Running pre-req install script ---")

	fmt.Println(string(output))
	if err != nil {
		return fmt.Errorf("Error running ssh to a VPS %s", err)
	}
	fmt.Println("--- End of pre-req install script  ---")

	pathToDComms := "/home/" + t.User + "/dcomms/setup"
	pathToDCommsInstall := pathToDComms + "/install.sh"

	if t.ContainerBackend == ContainerBackendPodman {
		pathToDCommsInstall = pathToDComms + "/install_podman.sh"
	}

	installCmd := fmt.Sprintf(`cd %s && chmod +x %s && PATH=$PATH:~/.local/bin DWEB_DOMAIN=%s CHOICES="%s" %s`, pathToDComms, pathToDCommsInstall, t.Domain, strings.Join(choices, " "), pathToDCommsInstall)

	// FIXME: temporary PATH setting
	if t.Remote {
		if err = sshExecCommand(ctx, t, installCmd); err != nil {
			return fmt.Errorf("Failed to exec install script: %s", err)
		}
	} else {
		if err = execCommand(ctx, t, installCmd); err != nil {
			return fmt.Errorf("Failed to exec install script: %s", err)
		}
	}

	return nil
}
