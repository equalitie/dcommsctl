package main

import (
	"fmt"
	"context"

	// "go.podman.io/podman/v6/pkg/bindings"
	"github.com/compose-spec/compose-go/v2/cli"
)

// there are no official bindings to work with podman-compose from go, but we can manually parse the compose format
// there might be diffuculties merge multiple docker-compose files but at least we can

func compose(path string) error {
	ctx := context.Background()
	opt, err := cli.NewProjectOptions(
		[]string{path},
		cli.WithOsEnv,
		cli.WithDotEnv,
		cli.WithName("proj"),
	)

	if err != nil {
		return fmt.Errorf("Composer parser options err: %s", err)
	}

	prj, err := opt.LoadProject(ctx)

	if err != nil {
		return err
	}

	srvs, err := prj.GetServices() 

	if err != nil {
		return nil
	}

	fmt.Println("Printing services from a compose file", path)

	for _, s := range srvs {
		fmt.Println(s.Name, s.ContainerName)
	}

	return nil
}

func testAllServices(app Application) {
	// FIXME: that's for local paths right now, we need to work on actually reading it from the OS, not letting the module read it
	for _, srv := range app.systemCfg.Services {
		if err := compose(srv.ComposePath); err != nil {
			fmt.Println(err)
		}
	}
}

