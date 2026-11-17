// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package cli is the amp command line shared by every distribution.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/wso2/agent-manager/agent-manager-service/ampinit"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/unsupported"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// errUsage asks for the usage text and exit code 2.
var errUsage = errors.New("usage")

const usage = `Usage:
  %[1]s serve [--migrate] [--wait-for-migrations]   run the API server
  %[1]s migrate                                      apply the core's and every compiled-in module's migrations
  %[1]s init <component> <dir> [<component> <dir>...]
                                                     write a component's configuration into dir and exit;
                                                     components: thunder, gateway, fileauth
  %[1]s module list                                  list the modules compiled into this binary
  %[1]s module install <name> [--with-deps]          install a module (and the providers it needs)
  %[1]s module enable <name>
  %[1]s module disable <name>
  %[1]s module uninstall <name>                      deactivate a module; its data is kept
  %[1]s version

Module changes take effect on running servers within seconds, without a restart.
The flags of earlier releases (-migrate, -server=false, -init-thunder=DIR, ...) still work.
`

// Main runs the command line. distribution names the binary.
func Main(distribution string) {
	err := run(distribution, os.Args[1:])
	if errors.Is(err, errUsage) {
		fmt.Fprintf(os.Stderr, usage, distribution)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(distribution string, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return legacy(args)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "migrate":
		if len(args) > 1 {
			return errUsage
		}
		runApp(app.Options{Migrate: true})
		return nil
	case "init":
		return initCmd(args[1:])
	case "module", "modules":
		return moduleCmd(args[1:])
	case "version":
		fmt.Printf("%s %s, core API v%d, modules:", distribution, config.Version, modkit.CoreAPIVersion)
		for _, m := range modkit.Registered() {
			fmt.Printf(" %s@%s", m.Manifest().Name, m.Manifest().Version)
		}
		fmt.Println()
		return nil
	case "help", "-h", "--help":
		return errUsage
	default:
		return errUsage
	}
}

// legacy keeps the flags of earlier releases working: no subcommand serves.
func legacy(args []string) error {
	fs := flag.NewFlagSet("amp", flag.ContinueOnError)
	serverFlag := fs.Bool("server", true, "start the http Server")
	migrateFlag := fs.Bool("migrate", false, "migrate the database")
	waitFlag := fs.Bool("wait-for-migrations", false, "wait until the database migrations are applied")
	initThunderDir := fs.String("init-thunder", "", "write ThunderID's configuration into this directory and exit")
	initGatewayDir := fs.String("init-gateway", "", "write the AI Gateway's configuration into this directory and exit")
	initFileAuthDir := fs.String("init-fileauth", "", "write the file-based sign-in user file into this directory and exit")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if *initThunderDir != "" || *initGatewayDir != "" || *initFileAuthDir != "" {
		return writeConfig(*initThunderDir, *initGatewayDir, *initFileAuthDir)
	}
	runApp(app.Options{Server: *serverFlag, Migrate: *migrateFlag, WaitForMigrations: *waitFlag})
	return nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	migrate := fs.Bool("migrate", false, "apply pending core and module migrations before serving")
	wait := fs.Bool("wait-for-migrations", false, "wait until another process has applied this build's migrations")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errUsage
	}
	runApp(app.Options{Server: true, Migrate: *migrate, WaitForMigrations: *wait})
	return nil
}

func runApp(opts app.Options) {
	// No secret store is bundled yet; secret writes report ErrNotSupported.
	secretProvider := unsupported.NewProvider()
	// DB-backed AgentID provisioning: the environment Thunder credential is
	// read from the database, not a key vault.
	opts.AgentThunderProvisioning = services.DBBackedAgentThunderProvisioning()
	app.Run(secretProvider, opts)
}

func initCmd(args []string) error {
	if len(args) == 0 || len(args)%2 != 0 {
		return errUsage
	}
	dirs := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		switch args[i] {
		case "thunder", "gateway", "fileauth":
			dirs[args[i]] = args[i+1]
		default:
			return fmt.Errorf("unknown component %q (want thunder, gateway or fileauth)", args[i])
		}
	}
	return writeConfig(dirs["thunder"], dirs["gateway"], dirs["fileauth"])
}

func writeConfig(thunderDir, gatewayDir, fileAuthDir string) error {
	if err := ampinit.Run(thunderDir, gatewayDir, fileAuthDir); err != nil {
		return fmt.Errorf("writing component configuration failed: %w", err)
	}
	return nil
}

func moduleCmd(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	withDeps := false
	var pos []string
	for _, a := range args[1:] {
		switch a {
		case "--with-deps", "-with-deps":
			withDeps = true
		default:
			if strings.HasPrefix(a, "-") {
				return errUsage
			}
			pos = append(pos, a)
		}
	}
	if withDeps && args[0] != "install" {
		return errUsage
	}
	want := 1
	if args[0] == "list" {
		want = 0
	}
	if len(pos) != want {
		return errUsage
	}

	// Lifecycle progress is printed below; keep the host's own logs to warnings.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	ctx := context.Background()
	h := modhost.New(db.GetDB(), config.GetConfig().Database.Driver, slog.Default())
	if err := h.CheckSchema(ctx); err != nil {
		return err
	}
	if err := h.Reload(ctx); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return list(os.Stdout, h.List())
	case "install":
		steps, err := h.Install(ctx, pos[0], withDeps)
		if err != nil {
			return err
		}
		for _, s := range steps {
			fmt.Printf("%sed %s\n", strings.TrimSuffix(s.Action, "e"), s.Module)
		}
		return nil
	case "enable":
		return report(h.Enable(ctx, pos[0]), "enabled", pos[0])
	case "disable":
		return report(h.Disable(ctx, pos[0]), "disabled", pos[0])
	case "uninstall":
		return report(h.Uninstall(ctx, pos[0]), "uninstalled", pos[0])
	default:
		return errUsage
	}
}

func report(err error, done, name string) error {
	if err != nil {
		return err
	}
	fmt.Println(done, name)
	return nil
}

func list(w io.Writer, mods []modhost.ModuleInfo) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// Errors surface from Flush: the tabwriter buffers every line until then.
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tSTATE\tBUILTIN\tPROVIDES\tREQUIRES")
	for _, m := range mods {
		builtin := ""
		if m.Builtin {
			builtin = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Name, m.Version, m.State, builtin,
			dash(strings.Join(m.Provides, ",")), dash(strings.Join(m.Requires, ",")))
	}
	return tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
