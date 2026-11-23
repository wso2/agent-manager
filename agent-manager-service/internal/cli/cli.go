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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/wso2/agent-manager/agent-manager-service/ampinit"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/inhouse"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/unsupported"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// errUsage asks for the usage text and exit code 2.
var errUsage = errors.New("usage")

const usage = `Usage: %[1]s [--config amp.yaml] <command>

  %[1]s serve [--migrate] [--wait-for-migrations]   run the API server
  %[1]s migrate                                      apply the core's and every compiled-in module's migrations
  %[1]s init <component> <dir> [<component> <dir>...]
                                                     write a component's configuration into dir and exit;
                                                     components: secrets, thunder, fileauth, and those
                                                     of the compiled-in modules (ai-gateway)
  %[1]s module list                                  list the modules compiled into this binary
  %[1]s module install <name> [--with-deps]          install a module (and the providers it needs)
  %[1]s module enable <name>
  %[1]s module disable <name>
  %[1]s module uninstall <name> [--purge]            deactivate a module; its data is kept, or with
                                                     --purge emptied (tables, secrets, workload credentials)
  %[1]s module credential <name> <workload> [--subject S] [--out FILE]
                                                     mint a credential for a module's service workload; it is
                                                     printed once, or written to FILE (kept if FILE exists)
  %[1]s module health [<name>]                       report ok, degraded or inactive, with connected workloads
  %[1]s secret list                                  list the generated secrets (names only)
  %[1]s secret show <name>                           print a generated secret, such as admin-password
  %[1]s version

Module changes take effect on running servers within seconds, without a restart.
Settings come from the environment and, for anything it leaves unset, from the
configuration file: --config, else $AMP_CONFIG, else ./amp.yaml if present.
A modules: list there (or $AMP_MODULES) is applied by the server at boot.
The flags of earlier releases (-migrate, -server=false, -init-thunder=DIR, ...) still work.
`

// Main runs the command line. distribution names the binary; defaultModules
// are its modules the server installs at boot when no modules list is
// declared, unless they were installed before.
func Main(distribution string, defaultModules ...string) {
	defaults = defaultModules
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
	args, path, err := configFlag(args)
	if err != nil {
		return err
	}
	// Settings: the environment, then the configuration file, then defaults.
	// amp init runs before the server's settings are complete.
	// So does amp serve, which generates what is missing before checking.
	loadConfig := config.Load
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || args[0] == "init" || args[0] == "serve" || args[0] == "secret" {
		loadConfig = config.LoadUnchecked
	}
	if err := loadConfig(path); err != nil {
		return err
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return legacy(args, path)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:], path)
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
	case "secret", "secrets":
		return secretCmd(args[1:], os.Stdout)
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

// configFlag takes a leading --config PATH (or --config=PATH) off args.
func configFlag(args []string) ([]string, string, error) {
	if len(args) == 0 {
		return args, "", nil
	}
	for _, f := range []string{"--config", "-config"} {
		if v, ok := strings.CutPrefix(args[0], f+"="); ok {
			return args[1:], v, nil
		}
		if args[0] == f {
			if len(args) < 2 {
				return nil, "", errUsage
			}
			return args[2:], args[1], nil
		}
	}
	return args, "", nil
}

// legacy keeps the flags of earlier releases working: no subcommand serves.
func legacy(args []string, configPath string) error {
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
		others := map[string]string{}
		if *initGatewayDir != "" {
			others["ai-gateway"] = *initGatewayDir
		}
		return writeConfig("", *initThunderDir, *initFileAuthDir, others)
	}
	if err := prepareServer(configPath); err != nil {
		return err
	}
	runApp(app.Options{Server: *serverFlag, Migrate: *migrateFlag, WaitForMigrations: *waitFlag})
	return nil
}

func serve(args []string, configPath string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	migrate := fs.Bool("migrate", false, "apply pending core and module migrations before serving")
	wait := fs.Bool("wait-for-migrations", false, "wait until another process has applied this build's migrations")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errUsage
	}
	if err := prepareServer(configPath); err != nil {
		return err
	}
	runApp(app.Options{Server: true, Migrate: *migrate, WaitForMigrations: *wait})
	return nil
}

// prepareServer generates what the server needs and nothing set, then loads
// the settings again, now checked.
func prepareServer(configPath string) error {
	if err := firstRun(context.Background(), databaseHasData); err != nil {
		return err
	}
	return config.Load(configPath)
}

// databaseHasData reports whether the configured database already holds an
// install. A SQLite file that does not exist yet holds none.
func databaseHasData(ctx context.Context) (bool, error) {
	cfg := config.Unchecked()
	if cfg.Database.Driver == config.DBDriverSQLite {
		if _, err := os.Stat(cfg.Database.SQLitePath); errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
	}
	conn, err := db.Open(cfg.Database, cfg.POSTGRESQL)
	if err != nil {
		return false, err
	}
	if sqlDB, err := conn.DB(); err == nil {
		defer func() { _ = sqlDB.Close() }()
	}
	return dbmigrations.HasHistory(ctx, conn)
}

// defaults are the distribution's default modules.
var defaults []string

func runApp(opts app.Options) {
	opts.DefaultModules = defaults
	secretProvider, err := secretProviderFor(config.GetConfig().SecretManager.Provider)
	if err != nil {
		slog.Error("invalid secret store configuration", "error", err)
		os.Exit(1)
	}
	// DB-backed AgentID provisioning: the environment Thunder credential is
	// read from the database, not a key vault.
	opts.AgentThunderProvisioning = services.DBBackedAgentThunderProvisioning()
	app.Run(secretProvider, opts)
}

// secretProviderFor is the secret store SECRET_MANAGER_PROVIDER names: "inhouse" keeps secrets in the agent manager
// database behind the secret vault, and "unsupported" refuses every secret write. Any other name is refused, so a
// typo never silently changes where secrets go.
func secretProviderFor(name string) (secretmanagersvc.Provider, error) {
	switch strings.TrimSpace(name) {
	case inhouse.ProviderName:
		return inhouse.NewProvider(), nil
	case unsupported.ProviderName:
		return unsupported.NewProvider(), nil
	default:
		return nil, fmt.Errorf("SECRET_MANAGER_PROVIDER %q is not one of %q and %q", name, inhouse.ProviderName, unsupported.ProviderName)
	}
}

func initCmd(args []string) error {
	if len(args) == 0 || len(args)%2 != 0 {
		return errUsage
	}
	known := ampinit.Components()
	dirs := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		if !slices.Contains(known, args[i]) {
			return fmt.Errorf("unknown component %q (want %s)", args[i], strings.Join(known, ", "))
		}
		dirs[args[i]] = args[i+1]
	}
	secrets, thunder, fileAuth := dirs["secrets"], dirs["thunder"], dirs["fileauth"]
	delete(dirs, "secrets")
	delete(dirs, "thunder")
	delete(dirs, "fileauth")
	return writeConfig(secrets, thunder, fileAuth, dirs)
}

func writeConfig(secretsDir, thunderDir, fileAuthDir string, others map[string]string) error {
	if err := ampinit.Run(secretsDir, thunderDir, fileAuthDir, others); err != nil {
		return fmt.Errorf("writing component configuration failed: %w", err)
	}
	return nil
}

// moduleArgs are the arguments of a module command: positional ones, and
// the flags each command takes.
type moduleArgs struct {
	pos      []string
	withDeps bool
	purge    bool
	subject  string
	out      string
}

func parseModuleArgs(cmd string, args []string) (moduleArgs, error) {
	var a moduleArgs
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		value = strings.TrimSpace(value)
		takeValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", errUsage
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case (name == "--with-deps" || name == "-with-deps") && !hasValue && cmd == "install":
			a.withDeps = true
		case (name == "--purge" || name == "-purge") && !hasValue && cmd == "uninstall":
			a.purge = true
		case name == "--subject" && cmd == "credential":
			a.subject, err = takeValue()
		case name == "--out" && cmd == "credential":
			a.out, err = takeValue()
		case strings.HasPrefix(args[i], "-"):
			return a, errUsage
		default:
			a.pos = append(a.pos, args[i])
		}
		if err != nil {
			return a, err
		}
	}
	want := map[string][2]int{"list": {0, 0}, "health": {0, 1}, "credential": {2, 2}}
	n, ok := want[cmd]
	if !ok {
		n = [2]int{1, 1}
	}
	if len(a.pos) < n[0] || len(a.pos) > n[1] {
		return a, errUsage
	}
	return a, nil
}

func moduleCmd(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	a, err := parseModuleArgs(args[0], args[1:])
	if err != nil {
		return err
	}
	pos := a.pos

	// Lifecycle progress is printed below; keep the host's own logs to warnings.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	ctx := context.Background()
	h := modhost.New(db.GetDB(), config.GetConfig().Database.Driver, slog.Default())
	// The servers run the modules; this process only changes their state.
	h.Passive = true
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
		steps, err := h.Install(ctx, pos[0], a.withDeps)
		// A failed catch-up still installs: report the steps, then the error.
		for _, s := range steps {
			fmt.Printf("%sed %s\n", strings.TrimSuffix(s.Action, "e"), s.Module)
		}
		return err
	case "enable":
		return report(h.Enable(ctx, pos[0]), "enabled", pos[0])
	case "disable":
		return report(h.Disable(ctx, pos[0]), "disabled", pos[0])
	case "uninstall":
		if a.purge {
			return report(h.Uninstall(ctx, pos[0], true), "uninstalled and purged", pos[0])
		}
		return report(h.Uninstall(ctx, pos[0], false), "uninstalled", pos[0])
	case "credential":
		return credential(ctx, h, pos[0], pos[1], a)
	case "health":
		return health(ctx, os.Stdout, h, pos)
	default:
		return errUsage
	}
}

// credential mints a workload credential and prints it once, or writes it
// to --out. An --out file that already holds a credential is kept, so a
// bootstrap step can run on every start.
func credential(ctx context.Context, h *modhost.Host, module, workload string, a moduleArgs) error {
	if a.out != "" {
		if st, err := os.Stat(a.out); err == nil && st.Size() > 0 {
			fmt.Fprintf(os.Stderr, "kept the credential already in %s\n", a.out)
			return nil
		}
	}
	subject := a.subject
	if subject == "" {
		subject = workload
	}
	recorder := wiring.ProvideAuditRecorder(*config.GetConfig(), slog.Default())
	defer func() { _ = recorder.Close(context.Background()) }()
	ctx = audit.WithRecorder(ctx, recorder)
	ctx, _ = audit.NewRequestScope(ctx)
	ctx = audit.WithSource(ctx, audit.Source{Surface: audit.SurfaceSystem, ActorType: audit.ActorSystem, ActorID: "cli:amp module credential"})
	cred, err := h.IssueCredential(ctx, module, workload, subject)
	if err != nil {
		return err
	}
	if a.out == "" {
		fmt.Fprintf(os.Stderr, "credential %s for %s/%s (subject %s); it is shown only once:\n", cred.ID, module, workload, subject)
		fmt.Println(cred.Secret)
		return nil
	}
	if err := writeSecret(a.out, cred.Secret+"\n"); err != nil {
		if rerr := h.RevokeCredential(ctx, module, cred.ID); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote credential %s for %s/%s (subject %s) to %s\n", cred.ID, module, workload, subject, a.out)
	return nil
}

// writeSecret writes a file only its owner can read, atomically.
func writeSecret(path, data string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// health prints the health of one module, or of every installed one.
func health(ctx context.Context, w io.Writer, h *modhost.Host, names []string) error {
	if len(names) == 0 {
		for _, m := range h.List() {
			if m.State != modhost.StateAvailable {
				names = append(names, m.Name)
			}
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "MODULE\tSTATUS\tWORKLOAD\tINSTANCES\tREASONS")
	var details []modhost.Health
	for _, n := range names {
		hl, err := h.Health(ctx, n)
		if err != nil {
			return err
		}
		reasons := dash(strings.Join(hl.Reasons, "; "))
		if len(hl.Workloads) == 0 {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t-\t-\t%s\n", n, hl.Status, reasons)
		}
		for i, wl := range hl.Workloads {
			var inst []string
			for _, in := range wl.Instances {
				inst = append(inst, in.Instance+"@"+in.Subject+" ("+in.Protocol+")")
			}
			label := wl.Name
			if wl.Required {
				label += " (required)"
			}
			if i > 0 {
				reasons = ""
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", n, hl.Status, label, dash(strings.Join(inst, ", ")), reasons)
		}
		if len(hl.Detail) > 0 {
			details = append(details, hl)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, hl := range details {
		buf, err := json.MarshalIndent(hl.Detail, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "\n%s reports:\n%s\n", hl.Module, buf)
	}
	return nil
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
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tSTATE\tPROVIDES\tREQUIRES")
	for _, m := range mods {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Name, m.Version, m.State,
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

// secretCmd lists the generated secrets in the secrets directory, or prints
// one of them.
func secretCmd(args []string, w io.Writer) error {
	dir := config.SecretsDir()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	switch {
	case len(args) == 1 && args[0] == "list":
		for _, n := range names {
			if _, err := fmt.Fprintln(w, n); err != nil {
				return err
			}
		}
		return nil
	case len(args) == 2 && args[0] == "show":
		if !slices.Contains(names, args[1]) {
			return fmt.Errorf("no generated secret %q in %s (have: %s)", args[1], dir, strings.Join(names, ", "))
		}
		data, err := os.ReadFile(filepath.Join(dir, args[1]))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, strings.TrimRight(string(data), "\r\n"))
		return err
	}
	return errUsage
}
