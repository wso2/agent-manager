package instance

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type UseOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)

	Name string
}

type UseResult struct {
	Instance string `json:"instance"`
}

func NewUseCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &UseOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			return runUse(opts)
		},
	}
}

func runUse(o *UseOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	if _, ok := cfg.Instances[o.Name]; !ok {
		return render.Error(o.IO, scope, clierr.Newf(clierr.NoInstance, "instance %q not found in config", o.Name))
	}

	cfg.CurrentInstance = o.Name
	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	scope.Instance = o.Name

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, UseResult{Instance: o.Name})
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Switched to instance %s\n", cs.SuccessIcon(), o.Name)
	return nil
}
