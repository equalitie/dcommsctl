// this is a file with functions written at different stages for stuff that did not end up in the project
// some of this might come back, some not, but these are just reference implementations of ideas I was toying with
// this is not supposed to be a compileable file

func debugInfo(app Application) {
	listRes, err := containers.List(app.ctx, &containers.ListOptions{})

	if err != nil {
		fmt.Println("List error", err)
		os.Exit(1)
	}

	for _, c := range listRes {
		fmt.Println("Container name: ", c.Names[0])
	}
}

func httpGetBackup(app Application, w http.ResponseWriter, r *http.Request) {
	backupScriptBuild(app)
}

func backupScriptBuild(app Application) ([]byte, error) {
	var buf bytes.Buffer

	for _, service := range app.systemCfg.Services {
		for _, path := range service.ConfigFiles {
			buf.WriteString(path.Path + "\n")
		}

	}

	return buf.Bytes(), nil
}

func sshConnect(c ConnectionInfo) error {

	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)

	if err != nil {
		fmt.Errorf("unable to connect to ssh-agent socket: %v", err)
	}

	agentClient := agent.NewClient(conn)

	config := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", c.Host+":"+c.Port, config)

	if err != nil {
		return err
	}
	defer client.Close()


	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b

	if err := session.Run("touch ~/dcomms/test"); err != nil {
		return err
	}

	sftpClient, err := sftp.NewClient(client)

	if err != nil {
		return fmt.Errorf("Failed to init sftp connection %s", err)
	}

	if err := sftpCopy(sftpClient, "./dcomms-docker-compose/conf/compose/synapse.docker-compose.yml", "/home/dcomms/dcomms/synapse.docker-compose.yml"); err != nil {
		return fmt.Errorf("Failed to copy file: %s", err)
	}

	fmt.Println(b.String())

	return nil
}

func pullImages(ctx context.Context, cfg SystemConfig) error {
	if cfg.Peertube {
		_, err := images.Pull(ctx, "postgres:18", nil)
		if err != nil {
			return err
		}

		_, err = images.Pull(ctx, "chocobozzz/peertube:production-bookworm", nil)
		if err != nil {
			return err
		}
	}

	return nil
}

