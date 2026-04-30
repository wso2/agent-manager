package instance

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/prompter"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type RemoveOptions struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter
	Config   func() (*config.Config, error)

	Name string
	Yes  bool
}

type RemoveResult struct {
	Instance string `json:"instance"`
	Removed  bool   `json:"removed"`
}

func NewRemoveCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &RemoveOptions{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
		Config:   f.Config,
	}
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			return runRemove(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runRemove(o *RemoveOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	if _, ok := cfg.Instances[o.Name]; !ok {
		return render.Error(o.IO, scope, clierr.Newf(clierr.NoInstance, "instance %q not found in config", o.Name))
	}

	if !o.Yes {
		if !o.IO.CanPrompt() {
			return render.Error(o.IO, scope, clierr.New(clierr.ConfirmationRequired, "deletion requires --yes when stdin is not a terminal"))
		}
		if err := o.Prompter.ConfirmDeletion(o.Name); err != nil {
			return render.Error(o.IO, scope, clierr.Newf(clierr.ConfirmationRequired, "%v", err))
		}
	}

	delete(cfg.Instances, o.Name)
	if cfg.CurrentInstance == o.Name {
		cfg.CurrentInstance = ""
	}

	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, RemoveResult{Instance: o.Name, Removed: true})
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Removed instance %s\n", cs.SuccessIcon(), o.Name)
	return nil
}
