package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func sftpCopy(client *sftp.Client, srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("Failed to read: %s", err)
	}
	defer srcFile.Close()

	dstFile, err := client.Create(dstPath)
	if err != nil {
		return fmt.Errorf("Failed to create: %s", err)
	}
	defer dstFile.Close()

	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return fmt.Errorf("Failed to write: %s", err)
	}

	return nil
}

func copyFolder(client *sftp.Client, src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return client.MkdirAll(dstPath)
		}

		if err := client.MkdirAll(filepath.Dir(dstPath)); err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := client.Create(dstPath)
		if err != nil {
			return fmt.Errorf("Create err: %s", err)
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

func sshCopyProject(target DeploymentTargetInfo) error {
	fmt.Println("Starting copy")
	pathToDcomms := os.Getenv("DCOMMS_GIT_REPO_PATH")

	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: target.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", target.Host+":"+target.Port, config)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("Failed to init sftp connection: %s", err)
	}
	defer sftpClient.Close()

	if err := copyFolder(sftpClient, pathToDcomms, "/home/"+target.User+"/dcomms/dcomms-docker-compose"); err != nil {
		return fmt.Errorf("Fail to copy files: %s", err)
	}

	fmt.Println("Ending copy")
	return nil
}

func sshTunnel(ctx context.Context, t DeploymentTargetInfo, localAddr string) error {
	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("Unable to connect to ssh-agent socket: %s", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	remoteSSHClient, err := ssh.Dial("tcp", t.Host+":"+t.Port, config)
	if err != nil {
		return fmt.Errorf("Unable to dial ssh: %v", err)
	}
	// FIXME: when are we closing this?
	// defer remoteSSHClient.Close()
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("Failed to listen on local address: %s", err)
	}

	go func() {
		idleTimer := time.NewTimer(10 * time.Second)
		idleTimer.Stop()

		resetIdleTimer := func() {
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(10 * time.Second)
		}

		go func() {
			<-idleTimer.C
			fmt.Println("SSH tunnel idle. Closing...")
			remoteSSHClient.Close()
			listener.Close()
		}()

		for {
			localConn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept local connection: %s", err)
				continue
			}

			remoteConn, err := remoteSSHClient.Dial("unix", "/var/run/docker.sock")
			if err != nil {
				log.Printf("Failed to dial remote Docker socket: %s", err)
				localConn.Close()
				continue
			}

			go func() {
				defer localConn.Close()
				defer remoteConn.Close()
				io.Copy(localConn, remoteConn)
				resetIdleTimer()
			}()

			go func() {
				defer localConn.Close()
				defer remoteConn.Close()
				io.Copy(remoteConn, localConn)
				resetIdleTimer()
			}()
		}
	}()

	if err := sshExecCommand(ctx, t, `docker ps -a --format "table {{.Names}}\t{{.Status}}"`); err != nil {
		return fmt.Errorf("Failed to get data on compose containers: %s", err)
	}

	return nil
}

// FIXME: we need to get result out, either write to a buffer or something
func sshExecCommand(ctx context.Context, t DeploymentTargetInfo, cmd string) error {
	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", t.Host+":"+t.Port, config)
	if err != nil {
		return fmt.Errorf("unable to dial ssh: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create ssh session: %s", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)

	fmt.Println("---    Running ssh command    ---")
	fmt.Println(string(out))
	fmt.Println("--- End of ssh command output ---")

	if err != nil {
		return err
	}

	return nil
}

func remoteOsStatus(target DeploymentTargetInfo) []byte {
	var buf bytes.Buffer

	commands := []string{
		"grep MemTotal /proc/meminfo |tr -s ' ' |cut -d ' ' -f2",
		"free -h",
		"df -h",
		"uptime",
	}

	for _, cmd := range commands {
		if err := sshExecCommand(context.TODO(), target, cmd); err != nil {
			return nil
		}
	}

	return buf.Bytes()
}
