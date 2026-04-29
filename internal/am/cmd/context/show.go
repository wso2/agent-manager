package context

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type ShowOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

type ShowResult struct {
	URL string `json:"url"`
	Org string `json:"org,omitempty"`
}

func NewShowCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ShowOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	return &cobra.Command{
		Use:   "show",
		Short: "Show the current context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(opts)
		},
	}
}

func runShow(o *ShowOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	if cfg.CurrentInstance == "" {
		return render.Error(o.IO, scope, clierr.New(clierr.NoInstance, "no instance configured"))
	}

	inst, ok := cfg.Instances[cfg.CurrentInstance]
	if !ok {
		return render.Error(o.IO, scope, clierr.Newf(clierr.NoInstance, "current instance %q not found in config", cfg.CurrentInstance))
	}

	scope.Instance = cfg.CurrentInstance
	scope.Org = inst.CurrentOrg
	return render.Success(o.IO, scope, ShowResult{URL: inst.URL, Org: inst.CurrentOrg})
}
