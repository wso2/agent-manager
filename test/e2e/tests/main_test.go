package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is the shared API client used by all e2e tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

func TestMain(m *testing.M) {
	Cfg = framework.LoadConfig()

	if err := waitForReady(Cfg); err != nil {
		fmt.Fprintf(os.Stderr, "readiness gate failed: %v\n", err)
		os.Exit(1)
	}

	var err error
	Client, err = framework.NewAMPClient(Cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create API client: %v\n", err)
		os.Exit(1)
	}

	if err := verifyDefaultOrg(Client, Cfg.DefaultOrg); err != nil {
		fmt.Fprintf(os.Stderr, "default org verification failed: %v\n", err)
		os.Exit(1)
	}

	// Clean up stale e2e projects older than 1 hour before running tests.
	cleanupStaleE2EResources(Client, Cfg.DefaultOrg)

	// Clear log files from previous runs.
	framework.CleanLogDir()

	code := m.Run()

	// Print consolidated report from per-test log files.
	framework.PrintConsolidatedReport()

	os.Exit(code)
}

func waitForReady(cfg *framework.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ReadinessTimeout)
	defer cancel()

	url := cfg.AMPBaseURL + "/healthz"
	backoff := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("API not ready within %v", cfg.ReadinessTimeout)
		default:
		}

		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println("API is ready")
				return nil
			}
			fmt.Printf("healthz returned %d, retrying in %v...\n", resp.StatusCode, backoff)
		} else {
			fmt.Printf("healthz unreachable: %v, retrying in %v...\n", err, backoff)
		}

		time.Sleep(backoff)
		if backoff < 15*time.Second {
			backoff = backoff * 3 / 2
		}
	}
}

func verifyDefaultOrg(c *framework.AMPClient, orgName string) error {
	resp, err := c.Get("/api/v1/orgs")
	if err != nil {
		return fmt.Errorf("list orgs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list orgs returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read orgs response: %w", err)
	}

	var list framework.OrganizationListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("decode orgs response: %w", err)
	}

	for _, org := range list.Organizations {
		if org.Name == orgName {
			fmt.Printf("default org %q verified\n", orgName)
			return nil
		}
	}

	return fmt.Errorf("default org %q not found in %d organizations", orgName, list.Total)
}
