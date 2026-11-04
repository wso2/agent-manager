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

package testsetup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	dpops "github.com/wso2/agent-manager/test/e2e/operations/deploymentpipeline"
	envops "github.com/wso2/agent-manager/test/e2e/operations/environment"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

// SetupSharedITHelpdeskAgent provisions an IT helpdesk agent deployed in a
// single environment using a direct OpenAI key. It is idempotent and intended
// to be called from each consuming suite's BeforeSuite: the first call builds
// and deploys the agent; subsequent calls (in this or other suites) find it
// already active and reuse it, so the build happens exactly once.
//
// The returned SharedITHelpdeskAgent's EndpointURL/APIKey are freshly resolved
// on every call so each suite gets a usable invocation handle.
func SetupSharedITHelpdeskAgent(client *framework.AMPClient, cfg *framework.Config) *framework.SharedITHelpdeskAgent {
	return SetupITHelpdeskAgent(client, cfg, framework.SharedITHelpdeskAgentName,
		"Single-env IT helpdesk agent shared across agent/configuration/llmprovider domains")
}

// SetupITHelpdeskAgent provisions (idempotently) an IT helpdesk agent with the
// given name in the shared project and returns a usable invocation handle. The
// shared agent uses a well-known name; suites that mutate an agent's config (and
// must not poison the shared one) pass their own dedicated name instead.
func SetupITHelpdeskAgent(client *framework.AMPClient, cfg *framework.Config, agentName, description string) *framework.SharedITHelpdeskAgent {
	Expect(cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set for the IT helpdesk agent")

	agent := &framework.SharedITHelpdeskAgent{
		ProjectName: framework.E2ESharedProjectName,
		AgentName:   agentName,
	}

	EnsureProject(client, cfg, agent.ProjectName, "E2E Shared Project", "Shared project for e2e tests")
	agent.BuildName = ensureSharedAgentReady(client, cfg, agent.ProjectName, agent.AgentName, description)

	resolveEndpointAndKey(client, cfg, agent.ProjectName, agent.AgentName, cfg.DefaultEnv,
		&agent.EndpointURL, &agent.APIKey)
	agent.InvokeReq = framework.DefaultInvokeRequest()

	ginkgo.GinkgoWriter.Printf("IT helpdesk agent ready: project=%s agent=%s endpoint=%s\n",
		agent.ProjectName, agent.AgentName, agent.EndpointURL)
	return agent
}

// SynchronizedSharedITHelpdeskAgent returns the two phase functions for a
// SynchronizedBeforeSuite in a suite that reuses the shared single-env IT
// helpdesk agent.
//
// Under `ginkgo -p` a plain BeforeSuite runs on every parallel process, so the
// shared agent would be provisioned concurrently N times — producing the racy
// "already exists" creates and the self-heal delete/recreate that wedges the
// deployment into a failed state. SynchronizedBeforeSuite instead runs the first
// phase on a single process: it provisions the agent exactly once and returns it
// as JSON. The second phase runs on every process: it builds that process's own
// API client and decodes the shared agent handle. The provided pointers are
// populated by the second phase so the suite's package-level vars are set on
// every process.
func SynchronizedSharedITHelpdeskAgent(
	cfg **framework.Config,
	client **framework.AMPClient,
	agent **framework.SharedITHelpdeskAgent,
) (func() []byte, func([]byte)) {
	return SynchronizedITHelpdeskAgent(SetupSharedITHelpdeskAgent, cfg, client, agent)
}

// SynchronizedITHelpdeskAgent builds the two SynchronizedBeforeSuite phase
// functions around a provisioner that runs once on process #1. Phase 1 sets up a
// client, calls provision, and returns the resulting agent as JSON; phase 2 runs
// on every process to build that process's own client and decode the handle into
// the provided pointers. Use it directly with a custom provisioner (e.g. a
// suite-private agent name); SynchronizedSharedITHelpdeskAgent wraps it for the
// shared agent.
func SynchronizedITHelpdeskAgent(
	provision func(*framework.AMPClient, *framework.Config) *framework.SharedITHelpdeskAgent,
	cfg **framework.Config,
	client **framework.AMPClient,
	agent **framework.SharedITHelpdeskAgent,
) (func() []byte, func([]byte)) {
	return synchronizedAgentSetup(provision, cfg, client, agent)
}

// SynchronizedCLILifecycleAgent returns the two SynchronizedBeforeSuite phase
// functions for the amctl CLI platform-agent suite. Phase 1 provisions the
// dedicated CLI-owned agent exactly once on parallel process 1; phase 2 runs on
// every process to build its own API client and decode the shared handle.
//
// Wire it into the harness's single SynchronizedBeforeSuite via
// amctl.WithSharedSetup. Doing so keeps the CLI-owned agent from being created
// concurrently by each parallel process — the race that otherwise produced
// "already exists" creates and deployments wedged into a failed state.
func SynchronizedCLILifecycleAgent(
	cfg **framework.Config,
	client **framework.AMPClient,
	agent **framework.CLILifecycleAgent,
) (func() []byte, func([]byte)) {
	return synchronizedAgentSetup(SetupCLILifecycleAgent, cfg, client, agent)
}

// synchronizedAgentSetup builds the generic two-phase SynchronizedBeforeSuite
// functions around a provisioner that runs once on process 1. Phase 1 sets up a
// client, calls provision, and returns the resulting handle as JSON; phase 2
// runs on every process to build that process's own client and decode the
// handle into the provided pointers.
func synchronizedAgentSetup[T any](
	provision func(*framework.AMPClient, *framework.Config) *T,
	cfg **framework.Config,
	client **framework.AMPClient,
	agent **T,
) (func() []byte, func([]byte)) {
	phase1 := func() []byte {
		c := framework.LoadConfig()
		ginkgo.By("Waiting for API readiness")
		framework.WaitForAPIReady(c)
		ginkgo.By("Creating API client")
		cl, err := framework.NewAMPClient(c)
		Expect(err).NotTo(HaveOccurred(), "failed to create API client")
		ginkgo.By("Verifying default organization")
		framework.VerifyDefaultOrg(cl, c.DefaultOrg)
		data, err := json.Marshal(provision(cl, c))
		Expect(err).NotTo(HaveOccurred(), "failed to marshal agent")
		return data
	}

	phase2 := func(data []byte) {
		*cfg = framework.LoadConfig()
		framework.WaitForAPIReady(*cfg)
		cl, err := framework.NewAMPClient(*cfg)
		Expect(err).NotTo(HaveOccurred(), "failed to create API client")
		*client = cl
		a := new(T)
		Expect(json.Unmarshal(data, a)).To(Succeed(), "failed to decode agent")
		*agent = a
	}

	return phase1, phase2
}

// SetupCLILifecycleAgent provisions a dedicated IT-helpdesk agent owned solely
// by the amctl CLI e2e suite, in its own project. It mirrors
// SetupSharedITHelpdeskAgent (idempotent build-once + self-heal via
// ensureSharedAgentReady) but under CLI-specific names, so the suite can
// run mutating commands (deploy/redeploy, and future agent llm *) without
// disturbing the shared agent that the HTTP traces/monitors suites read.
func SetupCLILifecycleAgent(client *framework.AMPClient, cfg *framework.Config) *framework.CLILifecycleAgent {
	Expect(cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set for the CLI lifecycle agent")

	agent := &framework.CLILifecycleAgent{
		ProjectName: framework.E2ECLIProjectName,
		AgentName:   framework.CLILifecycleAgentName,
	}

	EnsureProject(client, cfg, agent.ProjectName, "E2E CLI Project", "Dedicated project for amctl CLI e2e mutation tests")
	agent.BuildName = ensureSharedAgentReady(client, cfg, agent.ProjectName, agent.AgentName,
		"Dedicated IT helpdesk agent owned by the amctl CLI e2e suite")

	resolveEndpointAndKey(client, cfg, agent.ProjectName, agent.AgentName, cfg.DefaultEnv,
		&agent.EndpointURL, &agent.APIKey)
	agent.InvokeReq = framework.DefaultInvokeRequest()

	ginkgo.GinkgoWriter.Printf("CLI lifecycle agent ready: project=%s agent=%s endpoint=%s\n",
		agent.ProjectName, agent.AgentName, agent.EndpointURL)
	return agent
}

// CleanupCLILifecycleAgent returns a best-effort teardown that deletes the
// CLI-owned agent and its project. The agent is owned solely by this suite, so
// it is safe to drop on exit. It deletes by constant name (not the provisioned
// handle) so cleanup runs even if provisioning partially failed. Wire it via
// amctl.WithSharedTeardown to run once on process 1 after all specs finish.
func CleanupCLILifecycleAgent(client **framework.AMPClient, cfg **framework.Config) func() {
	return func() {
		if client == nil || *client == nil || cfg == nil || *cfg == nil {
			ginkgo.GinkgoWriter.Println("teardown: CLI lifecycle agent cleanup skipped (client/config unset)")
			return
		}
		cl, c := *client, *cfg
		// Agent before project (LIFO), mirroring the per-test cleanup order.
		agentops.DeleteAgentBestEffort(cl, c.DefaultOrg, framework.E2ECLIProjectName, framework.CLILifecycleAgentName)
		deleteProjectBestEffort(cl, c.DefaultOrg, framework.E2ECLIProjectName)
	}
}

// deleteProjectBestEffort deletes a project without failing the suite. A 409
// (agent cascade still in flight) is retried; other non-2xx is logged and left
// for the stale sweep.
func deleteProjectBestEffort(client *framework.AMPClient, orgName, projName string) {
	projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, projName)
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}
		resp, err := client.Delete(projPath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("teardown: delete project %q failed: %v\n", projName, err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
			ginkgo.GinkgoWriter.Printf("teardown: deleted project %q (status %d)\n", projName, resp.StatusCode)
			return
		}
		if resp.StatusCode == http.StatusConflict && attempt < 4 {
			ginkgo.GinkgoWriter.Printf("teardown: project %q still has resources, retrying...\n", projName)
			continue
		}
		ginkgo.GinkgoWriter.Printf("teardown: delete project %q returned status %d (left for stale sweep)\n",
			projName, resp.StatusCode)
		return
	}
}

// SetupSharedPromotableITHelpdeskAgent provisions the shared promotable IT
// helpdesk agent: it ensures a second environment and a deployment pipeline
// exist, builds the agent, and deploys it to the default environment. It is
// idempotent — the first caller builds/deploys; subsequent callers reuse the
// active agent.
//
// Kind publishing and promotion to the second environment are performed by the
// consuming tests (the agent domain's promotion suite), not here; this setup
// only establishes the shared, default-environment baseline.
func SetupSharedPromotableITHelpdeskAgent(ctx context.Context, client *framework.AMPClient, cfg *framework.Config) *framework.SharedPromotableITHelpdeskAgent {
	Expect(cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set for the promotable IT helpdesk agent")

	agent := &framework.SharedPromotableITHelpdeskAgent{
		AgentName: framework.SharedPromotableITHelpdeskAgentName,
	}
	agent.ProjectName, agent.SecondEnv = EnsurePromotableInfra(ctx, client, cfg)
	agent.BuildName = ensureSharedAgentReady(client, cfg, agent.ProjectName, agent.AgentName,
		"Promotable IT helpdesk agent shared across agent/configuration/llmprovider domains")

	resolveEndpointAndKey(client, cfg, agent.ProjectName, agent.AgentName, cfg.DefaultEnv,
		&agent.EndpointURL, &agent.APIKey)
	agent.InvokeReq = framework.DefaultInvokeRequest()

	ginkgo.GinkgoWriter.Printf("Shared promotable IT helpdesk agent ready: project=%s agent=%s secondEnv=%s endpoint=%s\n",
		agent.ProjectName, agent.AgentName, agent.SecondEnv, agent.EndpointURL)
	return agent
}

// ensureSharedAgentReady makes the named IT helpdesk agent active in the default
// environment, idempotently, and returns its build name. It distinguishes the
// states a shared agent can be in:
//
//   - active                  → reuse as-is (no build/deploy).
//   - missing                 → create it (which triggers build + deploy).
//   - terminal error/failed   → self-heal: delete and recreate, restoring a
//     clean state instead of failing on every reuse for the lifetime of a
//     wedged agent.
//   - in progress (any other) → wait for the in-flight build/deploy to finish.
//
// After create/recreate/in-progress it waits for build success, an active
// deployment, and runtime readiness.
func ensureSharedAgentReady(client *framework.AMPClient, cfg *framework.Config, projName, agentName, description string) string {
	agentPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", cfg.DefaultOrg, projName, agentName)
	createReq := framework.NewITHelpdeskAgentRequest(agentName, description, map[string]string{
		"OPENAI_API_KEY": cfg.OpenAIAPIKey,
		"DATABASE_URL":   "http://localhost:5000",
	})

	status := deployment.DeploymentStatus(client, cfg.DefaultOrg, projName, agentName, cfg.DefaultEnv)
	switch {
	case status == "active":
		ginkgo.GinkgoWriter.Printf("Agent %q already active in %s, reusing\n", agentName, cfg.DefaultEnv)
		return ""

	case isTerminalDeploymentError(status):
		ginkgo.By(fmt.Sprintf("Agent %q is in terminal %q state; deleting and recreating", agentName, status))
		agentops.DeleteAgent(Default, client, cfg.DefaultOrg, projName, agentName)
		Eventually(func() bool {
			return !framework.ResourceExists(client, agentPath)
		}).WithTimeout(3*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
			"agent %q was not deleted in time", agentName)
		ginkgo.By(fmt.Sprintf("Recreating agent %q", agentName))
		agentops.CreateAgent(Default, client, &agentops.CreateAgentParams{
			OrgName: cfg.DefaultOrg, ProjectName: projName, Request: createReq,
		})

	case !framework.ResourceExists(client, agentPath):
		ginkgo.By(fmt.Sprintf("Creating agent %q", agentName))
		agentops.CreateAgent(Default, client, &agentops.CreateAgentParams{
			OrgName: cfg.DefaultOrg, ProjectName: projName, Request: createReq,
		})

	default:
		ginkgo.GinkgoWriter.Printf("Agent %q exists but is not active yet (status=%q); waiting\n", agentName, status)
	}

	ginkgo.By(fmt.Sprintf("Waiting for agent %q build", agentName))
	buildName := build.WaitForBuildSuccess(client, &build.WaitForBuildParams{
		OrgName:     cfg.DefaultOrg,
		ProjectName: projName,
		AgentName:   agentName,
		Timeout:     20 * time.Minute,
	})

	ginkgo.By(fmt.Sprintf("Waiting for agent %q deployment", agentName))
	deployment.WaitForDeployed(client, &deployment.WaitForDeploymentParams{
		OrgName:     cfg.DefaultOrg,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: cfg.DefaultEnv,
		Timeout:     5 * time.Minute,
	})

	ginkgo.By(fmt.Sprintf("Waiting for agent %q readiness", agentName))
	deployment.WaitForReadiness(client, cfg.DefaultOrg, projName, agentName, cfg.DefaultEnv, 10*time.Minute)

	return buildName
}

// isTerminalDeploymentError reports whether a deployment status represents a
// terminal failure (as opposed to a transient in-progress state).
func isTerminalDeploymentError(status string) bool {
	switch strings.ToLower(status) {
	case "error", "failed":
		return true
	}
	return false
}

// EnsurePromotableInfra idempotently provisions the shared infrastructure for
// the promotable IT helpdesk agent: the promotion-target environment and a
// project with a default→secondEnv deployment pipeline. It returns the project
// name and second environment name. It is safe to call from a test's BeforeAll
// (when that test owns the promotable agent's lifecycle) or from
// SetupSharedPromotableITHelpdeskAgent (when a reusing domain just needs the
// agent to exist).
func EnsurePromotableInfra(ctx context.Context, client *framework.AMPClient, cfg *framework.Config) (projName, secondEnv string) {
	projName = framework.E2ESharedProjectWithMultiEnvDepPipeline
	secondEnv = framework.E2ESharedSecondEnv
	ensureSecondEnvironment(ctx, client, cfg, secondEnv)
	ensurePromotableProject(client, cfg, projName, secondEnv)
	return projName, secondEnv
}

// EnsureProject creates the named project if it does not already exist and waits
// until it is visible. Project creation is asynchronous (the POST returns 202
// before the project is queryable), so callers that go on to create resources in
// the project must wait for it to appear; otherwise that follow-up create races
// the project's provisioning.
func EnsureProject(client *framework.AMPClient, cfg *framework.Config, name, displayName, description string) {
	projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", cfg.DefaultOrg, name)
	if framework.ResourceExists(client, projPath) {
		return
	}
	ginkgo.By(fmt.Sprintf("Creating project %q", name))
	project.CreateProject(Default, client, &project.CreateProjectParams{
		OrgName: cfg.DefaultOrg,
		Request: framework.NewCreateProjectRequest(name, displayName, description, "default"),
	})
	Eventually(func() bool {
		return framework.ResourceExists(client, projPath)
	}).WithTimeout(2*time.Minute).WithPolling(2*time.Second).Should(BeTrue(),
		"project %q was not visible in time after creation", name)
}

// ensureSecondEnvironment provisions the shared promotion-target environment via
// the add-environment script if it does not already exist.
func ensureSecondEnvironment(ctx context.Context, client *framework.AMPClient, cfg *framework.Config, envName string) {
	envPath := fmt.Sprintf("/api/v1/orgs/%s/environments/%s", cfg.DefaultOrg, envName)
	if framework.ResourceExists(client, envPath) {
		ginkgo.GinkgoWriter.Printf("Shared second environment already exists: %s\n", envName)
		return
	}
	ginkgo.By(fmt.Sprintf("Creating shared second environment %q", envName))
	scriptParams := envops.FromClient(client)
	scriptParams.EnvName = envName
	scriptParams.DisplayName = "E2E Shared Staging"
	// Agents are promoted into this env, which needs its own Thunder to mint
	// their identity — even when the run sets PROVISION_THUNDER=false globally.
	scriptParams.ProvisionThunder = true
	envops.AddEnvironmentWithContext(ctx, scriptParams)
}

// ensurePromotableProject creates the promotable project (with a
// default→secondEnv promotion pipeline) if it does not already exist.
func ensurePromotableProject(client *framework.AMPClient, cfg *framework.Config, projName, secondEnv string) {
	projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", cfg.DefaultOrg, projName)
	if framework.ResourceExists(client, projPath) {
		return
	}

	ginkgo.By("Creating promotion deployment pipeline")
	pipeline := dpops.CreateOrGet(Default, client, cfg.DefaultOrg,
		framework.CreateDeploymentPipelineRequest{
			DisplayName: "E2E Shared Promotion Pipeline",
			PromotionPaths: []framework.PromotionPath{
				{
					SourceEnvironmentRef: cfg.DefaultEnv,
					TargetEnvironmentRefs: []framework.TargetEnvironmentRef{
						{Name: secondEnv},
					},
				},
			},
		})

	ginkgo.By(fmt.Sprintf("Creating promotable project %q", projName))
	projReq := framework.NewCreateProjectRequest(projName, "E2E Promotable Project", "Shared promotable project for e2e tests", pipeline.Name)
	project.CreateProject(Default, client, &project.CreateProjectParams{
		OrgName: cfg.DefaultOrg,
		Request: projReq,
	})
}

// resolveEndpointAndKey fetches the agent's endpoint URL in the given
// environment and mints a fresh API key, storing them via the provided pointers.
func resolveEndpointAndKey(client *framework.AMPClient, cfg *framework.Config, projName, agentName, env string, endpointURL, apiKey *string) {
	endpoints := deployment.GetEndpoints(Default, client, cfg.DefaultOrg, projName, agentName, env)
	for _, ep := range endpoints {
		if ep.URL != "" {
			*endpointURL = ep.URL
			break
		}
	}
	Expect(*endpointURL).NotTo(BeEmpty(), "agent %q endpoint URL in env %q should not be empty", agentName, env)

	apiKeyResp := agentops.CreateAgentAPIKey(Default, client,
		cfg.DefaultOrg, projName, agentName, env,
		framework.CreateAgentAPIKeyRequest{
			DisplayName: "e2e-shared-key",
			ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
	*apiKey = apiKeyResp.ApiKey
	Expect(*apiKey).NotTo(BeEmpty(), "agent %q API key in env %q should not be empty", agentName, env)
}
