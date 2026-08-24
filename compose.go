package main

import (
	"context"
	"fmt"
	"os"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

func composeChangePorts(ctx context.Context, composeFilePath string, portMap map[string]string) (*types.Project, error) {
	opt, err := cli.NewProjectOptions(
		[]string{composeFilePath},
		cli.WithOsEnv,
		cli.WithDotEnv,
	)
	if err != nil {
		return nil, fmt.Errorf("Composer parser options err: %s", err)
	}

	prj, err := opt.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	prj, err = prj.WithServicesEnabled(prj.DisabledServiceNames()...)
	if err != nil {
		return nil, err
	}

	prj, err = prj.WithServicesTransform(
		func(name string, service types.ServiceConfig) (types.ServiceConfig, error) {
			for i := range service.Ports {
				published := service.Ports[i].Published
				if published == "" {
					continue
				}

				newPort, found := portMap[published]
				if !found {
					return service, fmt.Errorf("service %q port %q: %w", name, published, err)
				}
				service.Ports[i].Published = newPort
			}
			return service, nil
		},
	)

	return prj, nil
}

func composeStore(ctx context.Context, prj *types.Project, outFilePath string) error {
	outData, err := prj.MarshalYAML()
	if err != nil {
		return fmt.Errorf("encode compose file: %w", err)
	}

	f, err := os.OpenFile(outFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(outData); err != nil {
		return err
	}

	return nil
}
