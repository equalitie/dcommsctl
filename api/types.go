package api

import (
	"context"

	"eq/dcommsctl/internal/jobs"
)

type SystemConfig struct {
	DomainName   string
	SSODomain    string
	BaseDataPath string
	AppsDir      string
	InstancesDir string
	BackupsDir   string
	SkipDawa     bool
}

type InstanceGenericConfig struct {
	SSOIssuerURI string
	InstanceID   string
	ClientID     string
	ClientSecret string
	Domain       string
	OnionDomain  string
	Version      string
}

type GlobalState struct {
	ctx         context.Context
	SystemCfg   SystemConfig
	JobsManager *jobs.JobManager
}

func NewGlobalState(ctx context.Context) *GlobalState {
	return &GlobalState{
		JobsManager: jobs.NewJobManager(ctx),
	}
}
