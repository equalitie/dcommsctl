package main

import (
	"context"
	"fmt"
	"slices"
)

type ApplicationGetArgs struct {
	Id string `json:"id"`
}

// One catalog entry with the detail apps/list omits. The id is checked against appIDs
// before it reaches the filesystem, so it cannot walk out of conf_tmpl.
func rpcApplicationGet(ctx context.Context, args ApplicationGetArgs) (ApplicationDetail, error) {
	if !slices.Contains(appIDs, args.Id) {
		return ApplicationDetail{}, fmt.Errorf("unknown application: %s", args.Id)
	}

	app, err := loadAppConfig(args.Id)
	if err != nil {
		return ApplicationDetail{}, err
	}

	return app, nil
}
