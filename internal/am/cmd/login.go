package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/auth"
	"github.com/wso2/agent-manager/internal/am/config"
)

type LoginResult struct {
	Instance  string    `json:"instance"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewLoginCmd() *cobra.Command {
	var url string
	var name string
	var clientID string
	var clientSecret string
	var authServer string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to an instance",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" {
				return fmt.Errorf("--url is required")
			}
			if clientID == "" || clientSecret == "" {
				return fmt.Errorf("--client-id and --client-secret are required")
			}
			if name == "" {
				name = "default"
			}

			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}

			inst, err := auth.Login(cmd.Context(), auth.LoginOptions{
				URL:          url,
				Name:         name,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AuthServer:   authServer,
			})
			if err != nil {
				return err
			}

			cfg.AddInstance(name, *inst)
			if err := config.Save(path, *cfg); err != nil {
				return err
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(LoginResult{
				Instance:  name,
				URL:       inst.URL,
				ExpiresAt: inst.Auth.ExpiresAt,
			})
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "Agent Manager instance URL")
	cmd.Flags().StringVar(&name, "name", "", "Agent Manager instance name")
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret")
	cmd.Flags().StringVar(&authServer, "auth-server", "", "Authorization server base URL (e.g. http://thunder.amp.localhost:8080); skips OAuth metadata discovery and posts to <auth-server>/oauth2/token")

	return cmd
}
