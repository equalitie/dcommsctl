package main

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/cli"
)

func compose(ctx context.Context, composeFilePath string) error {
	opt, err := cli.NewProjectOptions(
		[]string{composeFilePath},
		cli.WithOsEnv,
		cli.WithDotEnv,
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

	fmt.Println("Printing services from a compose file", composeFilePath)

	for _, s := range srvs {
		fmt.Println(s.Name, s.ContainerName)
	}

	return nil
}
