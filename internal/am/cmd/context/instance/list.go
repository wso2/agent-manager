package instance

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type ListOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

type ListResult struct {
	Current   string         `json:"current"`
	Instances []InstanceItem `json:"instances"`
}

type InstanceItem struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	CurrentOrg string `json:"current_org,omitempty"`
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List configured instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(opts)
		},
	}
}

func runList(o *ListOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	var items []InstanceItem
	for name, inst := range cfg.Instances {
		items = append(items, InstanceItem{
			Name:       name,
			URL:        inst.URL,
			CurrentOrg: inst.CurrentOrg,
		})
	}
	if items == nil {
		items = []InstanceItem{}
	}

	return render.Success(o.IO, scope, ListResult{
		Current:   cfg.CurrentInstance,
		Instances: items,
	})
}
