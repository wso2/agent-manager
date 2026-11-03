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

package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpRenameRecorder captures every OpenChoreo write the rename makes, so a test can assert
// both what was written and that nothing was written at all.
type mcpRenameRecorder struct {
	removedFromComponent []string
	bindingRemoved       map[string][]string
	bindingVars          map[string][]client.EnvVar
	componentVars        []client.EnvVar
	writes               int
}

// ocClient returns an OpenChoreo mock whose rename-path writes are recorded on r.
func (r *mcpRenameRecorder) ocClient() *clientmocks.OpenChoreoClientMock {
	r.bindingRemoved = map[string][]string{}
	r.bindingVars = map[string][]client.EnvVar{}
	return &clientmocks.OpenChoreoClientMock{
		GetComponentBaseEnvVarsFunc: func(context.Context, string, string) ([]client.EnvVar, error) { return []client.EnvVar{}, nil },
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return []models.EnvVars{}, nil
		},
		RemoveComponentEnvironmentVariablesFunc: func(_ context.Context, _, _, _ string, keys []string) error {
			r.writes++
			r.removedFromComponent = keys
			return nil
		},
		ReplaceReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, envName string, keysToRemove []string, envVars []client.EnvVar) error {
			r.writes++
			r.bindingRemoved[envName] = keysToRemove
			r.bindingVars[envName] = envVars
			return nil
		},
		UpdateComponentEnvVarsFunc: func(_ context.Context, _, _, _ string, envVars []client.EnvVar) error {
			r.writes++
			r.componentVars = envVars
			return nil
		},
	}
}

// renamedTemplates is the env var templates after renaming to GITHUB_URL / GITHUB_API_KEY.
func renamedTemplates() []EnvConfigTemplate {
	return []EnvConfigTemplate{
		{Key: "url", Name: "GITHUB_URL"},
		{Key: "apikey", Name: "GITHUB_API_KEY", IsSecret: true},
	}
}

// renameVarRows builds one environment's url + apikey rows under the pre-rename names.
func renameVarRows(configUUID, envUUID uuid.UUID, secretRef string) []models.AgentEnvConfigVariable {
	return []models.AgentEnvConfigVariable{
		{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "url", VariableName: "GITHUB_MCP_URL"},
		{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "apikey", VariableName: "GITHUB_MCP_API_KEY", SecretReference: secretRef},
	}
}

// envVarByKey returns the env var named key, failing the test if it is absent.
func envVarByKey(t *testing.T, vars []client.EnvVar, key string) client.EnvVar {
	t.Helper()
	for _, v := range vars {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("env var %q not found in %+v", key, vars)
	return client.EnvVar{}
}

// Regression for wso2/agent-manager#2164: the agent-config repository loads mappings
// without the proxy's endpoint → environment bindings, so the rename resolved no gateway
// artifact, logged a warning and left every ReleaseBinding on the old names. The rename
// must resolve the gateway from the fully loaded proxy and rewrite each environment with
// its own URL and its own SecretReference under the "api-key" property.
func TestApplyRenamedMCPEnvVars_RewritesEachEnvWithItsOwnValues(t *testing.T) {
	f := newMCPURLTestFixture()
	stageEnv := uuid.New()
	configUUID := uuid.New()

	// The mapping as the agent-config repository used to hand it over: the proxy is
	// there, its Endpoints are not.
	bareMapping := f.mapping
	fullProxy := f.mapping.MCPProxy
	bareMapping.MCPProxy = &models.MCPProxy{UUID: fullProxy.UUID, Configuration: fullProxy.Configuration}

	rec := &mcpRenameRecorder{}
	svc := f.newService()
	svc.ocClient = rec.ocClient()
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return []models.EnvAgentMCPMapping{bareMapping}, nil
		},
	}
	svc.mcpProxyRepo = &repomocks.MCPProxyRepositoryMock{
		GetByUUIDFunc: func(_ context.Context, proxyUUID, _ string) (*models.MCPProxy, error) {
			require.Equal(t, fullProxy.UUID.String(), proxyUUID)
			return fullProxy, nil
		},
	}

	config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "it-support", Name: "github"}
	vars := append(renameVarRows(configUUID, f.envUUID, "github-default-proxy-secrets"), renameVarRows(configUUID, stageEnv, "")...)
	uuidToEnvName := map[string]string{f.envUUID.String(): "default", stageEnv.String(): "stage"}
	oldKeys := []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"}

	_, err := svc.applyRenamedMCPEnvVars(context.Background(), config, f.ouID, "default", "it-support",
		uuidToEnvName, "default", vars, renamedTemplates(), oldKeys)
	require.NoError(t, err)

	require.ElementsMatch(t, oldKeys, rec.removedFromComponent)
	require.ElementsMatch(t, oldKeys, rec.bindingRemoved["default"])
	require.ElementsMatch(t, oldKeys, rec.bindingRemoved["stage"])

	defaultURL := envVarByKey(t, rec.bindingVars["default"], "GITHUB_URL")
	require.Equal(t, "https://gw.example.com/proxy-ctx/mcp", defaultURL.Value)
	defaultKey := envVarByKey(t, rec.bindingVars["default"], "GITHUB_API_KEY")
	require.NotNil(t, defaultKey.ValueFrom)
	require.Equal(t, client.SecretKeyRef{Name: "github-default-proxy-secrets", Key: secretmanagersvc.SecretKeyAPIKey},
		*defaultKey.ValueFrom.SecretKeyRef)

	// stage has variable rows but no mapping: it gets the new names, empty — never the
	// default environment's URL or secret.
	stageURL := envVarByKey(t, rec.bindingVars["stage"], "GITHUB_URL")
	require.Empty(t, stageURL.Value)
	stageKey := envVarByKey(t, rec.bindingVars["stage"], "GITHUB_API_KEY")
	require.Nil(t, stageKey.ValueFrom)
	require.Empty(t, stageKey.Value)

	require.Equal(t, rec.bindingVars["default"], rec.componentVars, "the Component base must carry the first environment's renamed vars")
}

// A gateway that cannot be resolved must fail the rename before anything is written, so the
// caller's transaction rolls the DB rename back and the Component and ReleaseBindings are
// never left half on the old names and half on the new ones.
func TestApplyRenamedMCPEnvVars_ResolutionFailureWritesNothing(t *testing.T) {
	f := newMCPURLTestFixture()
	configUUID := uuid.New()

	rec := &mcpRenameRecorder{}
	svc := f.newService()
	svc.ocClient = rec.ocClient()
	svc.gatewayRepo = &repomocks.GatewayRepositoryMock{
		EnvironmentMappingExistsFunc: func(string, string) (bool, error) {
			return false, errors.New("db unavailable")
		},
	}
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return []models.EnvAgentMCPMapping{f.mapping}, nil
		},
	}

	config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "it-support", Name: "github"}
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), config, f.ouID, "default", "it-support",
		map[string]string{f.envUUID.String(): "default"}, "default",
		renameVarRows(configUUID, f.envUUID, "github-default-proxy-secrets"), renamedTemplates(),
		[]string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})

	require.Error(t, err)
	require.Zero(t, rec.writes, "no OpenChoreo resource may be touched once resolution has failed")
}

// The live rewrite runs while the caller holds the configuration's variable rows locked, so a
// hung OpenChoreo call must not hold that lock for as long as the client's own retries allow.
// The rewrite gives up at mcpRenameApplyTimeout with a deadline error, which rolls the rename
// back and releases the lock.
func TestApplyRenamedMCPEnvVars_HungOpenChoreoCallTimesOut(t *testing.T) {
	prev := mcpRenameApplyTimeout
	mcpRenameApplyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { mcpRenameApplyTimeout = prev })

	f := newMCPURLTestFixture()
	configUUID := uuid.New()

	rec := &mcpRenameRecorder{}
	oc := rec.ocClient()
	oc.ReplaceReleaseBindingEnvVarsFunc = func(ctx context.Context, _, _, _, _ string, _ []string, _ []client.EnvVar) error {
		<-ctx.Done() // a call that never answers on its own
		return ctx.Err()
	}
	svc := f.newService()
	svc.ocClient = oc
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return []models.EnvAgentMCPMapping{f.mapping}, nil
		},
	}

	config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "it-support", Name: "github"}
	start := time.Now()
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), config, f.ouID, "default", "it-support",
		map[string]string{f.envUUID.String(): "default"}, "default",
		renameVarRows(configUUID, f.envUUID, "github-default-proxy-secrets"), renamedTemplates(),
		[]string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second, "the rewrite must give up at its own deadline, not wait on the hung call")
}

// A later write can fail after earlier environments have already restarted with the
// new names. Restore those names and values before the DB transaction rolls back.
func TestApplyRenamedMCPEnvVars_PartialFailureRestoresLiveValues(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "write failure", true: "request canceled"}[cancelRequest], func(t *testing.T) {
			f := newMCPURLTestFixture()
			configUUID, stageEnv := uuid.New(), uuid.New()
			oldVars := []client.EnvVar{
				{Key: "GITHUB_MCP_URL", Value: "https://gw.example.com/proxy-ctx/mcp"},
				{Key: "GITHUB_MCP_API_KEY", ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{Name: "default-secrets", Key: "api-key"},
				}},
			}
			stageVars := []client.EnvVar{{Key: "GITHUB_MCP_URL"}, {Key: "GITHUB_MCP_API_KEY"}}
			unrelated := client.EnvVar{Key: "UNRELATED", Value: "keep"}
			component := append(append([]client.EnvVar{}, oldVars...), unrelated)
			bindings := map[string][]client.EnvVar{
				"default": append(append([]client.EnvVar{}, oldVars...), unrelated),
				"stage":   append(append([]client.EnvVar{}, stageVars...), unrelated),
			}
			merge := func(current []client.EnvVar, remove []string, add []client.EnvVar) []client.EnvVar {
				values := map[string]client.EnvVar{}
				for _, v := range current {
					values[v.Key] = v
				}
				for _, key := range remove {
					delete(values, key)
				}
				for _, v := range add {
					values[v.Key] = v
				}
				result := make([]client.EnvVar, 0, len(values))
				for _, v := range values {
					result = append(result, v)
				}
				return result
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writeErr := errors.New("stage update failed")
			failed := false
			oc := &clientmocks.OpenChoreoClientMock{
				GetComponentBaseEnvVarsFunc: func(context.Context, string, string) ([]client.EnvVar, error) { return []client.EnvVar{}, nil },
				GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
					return []models.EnvVars{}, nil
				},
				RemoveComponentEnvironmentVariablesFunc: func(ctx context.Context, _, _, _ string, keys []string) error {
					require.NoError(t, ctx.Err(), "rollback must survive request cancellation")
					component = merge(component, keys, nil)
					return nil
				},
				UpdateComponentEnvVarsFunc: func(ctx context.Context, _, _, _ string, vars []client.EnvVar) error {
					require.NoError(t, ctx.Err())
					component = merge(component, nil, vars)
					return nil
				},
				ReplaceReleaseBindingEnvVarsFunc: func(ctx context.Context, _, _, _, env string, keys []string, vars []client.EnvVar) error {
					require.NoError(t, ctx.Err())
					bindings[env] = merge(bindings[env], keys, vars)
					if env == "stage" && !failed {
						failed = true
						if cancelRequest {
							cancel()
						}
						// Model an ambiguous failure: the server applied the write, but
						// the client failed to receive its response.
						return writeErr
					}
					return nil
				},
			}
			svc := f.newService()
			svc.ocClient = oc
			svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
				ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
					return []models.EnvAgentMCPMapping{f.mapping}, nil
				},
			}
			config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "agent", Name: "github"}
			vars := append(renameVarRows(configUUID, f.envUUID, "default-secrets"), renameVarRows(configUUID, stageEnv, "")...)
			_, err := svc.applyRenamedMCPEnvVars(ctx, config, f.ouID, "default", "agent",
				map[string]string{f.envUUID.String(): "default", stageEnv.String(): "stage"}, "default",
				vars, renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
			require.ErrorIs(t, err, writeErr)
			require.True(t, failed)
			require.ElementsMatch(t, append(oldVars, unrelated), component)
			require.ElementsMatch(t, append(oldVars, unrelated), bindings["default"])
			require.ElementsMatch(t, append(stageVars, unrelated), bindings["stage"])
		})
	}
}

// A failed compensation must be reported alongside the original write failure,
// while restoration of the remaining resources still proceeds.
func TestApplyRenamedMCPEnvVars_RollbackFailureStillRestoresOtherResources(t *testing.T) {
	f := newMCPURLTestFixture()
	configUUID, stageEnv := uuid.New(), uuid.New()
	writeErr, rollbackErr := errors.New("write failed"), errors.New("rollback failed")
	rec := &mcpRenameRecorder{}
	oc := rec.ocClient()
	recordBinding := oc.ReplaceReleaseBindingEnvVarsFunc
	stageCalls := 0
	oc.ReplaceReleaseBindingEnvVarsFunc = func(ctx context.Context, org, project, agent, env string, keys []string, vars []client.EnvVar) error {
		if env == "stage" {
			stageCalls++
			if stageCalls == 1 {
				return writeErr
			}
			return rollbackErr
		}
		return recordBinding(ctx, org, project, agent, env, keys, vars)
	}
	svc := f.newService()
	svc.ocClient = oc
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return []models.EnvAgentMCPMapping{f.mapping}, nil
		},
	}
	config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "agent", Name: "github"}
	vars := append(renameVarRows(configUUID, f.envUUID, "default-secrets"), renameVarRows(configUUID, stageEnv, "")...)
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), config, f.ouID, "default", "agent",
		map[string]string{f.envUUID.String(): "default", stageEnv.String(): "stage"}, "default",
		vars, renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
	require.ErrorIs(t, err, writeErr)
	require.ErrorIs(t, err, rollbackErr)
	require.Equal(t, 2, stageCalls)
	require.Equal(t, "https://gw.example.com/proxy-ctx/mcp", envVarByKey(t, rec.bindingVars["default"], "GITHUB_MCP_URL").Value)
	require.Equal(t, rec.bindingVars["default"], rec.componentVars)
}

func TestApplyRenamedMCPEnvVars_HungRollbackRestoresHealthyResources(t *testing.T) {
	prev := mcpRenameApplyTimeout
	mcpRenameApplyTimeout = 100 * time.Millisecond
	t.Cleanup(func() { mcpRenameApplyTimeout = prev })
	f := newMCPURLTestFixture()
	configUUID, stageEnv := uuid.New(), uuid.New()
	writeErr := errors.New("stage write failed")
	rec := &mcpRenameRecorder{}
	oc := rec.ocClient()
	recordBinding, recordRemove, recordComponent := oc.ReplaceReleaseBindingEnvVarsFunc, oc.RemoveComponentEnvironmentVariablesFunc, oc.UpdateComponentEnvVarsFunc
	stageCalls := 0
	oc.ReplaceReleaseBindingEnvVarsFunc = func(ctx context.Context, org, project, agent, env string, keys []string, vars []client.EnvVar) error {
		if env == "stage" {
			stageCalls++
			if stageCalls == 1 {
				return writeErr
			}
			<-ctx.Done()
			return ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return recordBinding(ctx, org, project, agent, env, keys, vars)
	}
	oc.RemoveComponentEnvironmentVariablesFunc = func(ctx context.Context, org, project, agent string, keys []string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return recordRemove(ctx, org, project, agent, keys)
	}
	oc.UpdateComponentEnvVarsFunc = func(ctx context.Context, org, project, agent string, vars []client.EnvVar) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return recordComponent(ctx, org, project, agent, vars)
	}
	svc := f.newService()
	svc.ocClient = oc
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return []models.EnvAgentMCPMapping{f.mapping}, nil
		},
	}
	config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "agent", Name: "github"}
	vars := append(renameVarRows(configUUID, f.envUUID, "default-secrets"), renameVarRows(configUUID, stageEnv, "")...)
	start := time.Now()
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), config, f.ouID, "default", "agent",
		map[string]string{f.envUUID.String(): "default", stageEnv.String(): "stage"}, "default",
		vars, renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
	require.ErrorIs(t, err, writeErr)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, 2, stageCalls)
	require.ElementsMatch(t, []string{"GITHUB_URL", "GITHUB_API_KEY"}, rec.bindingRemoved["default"])
	require.Equal(t, "https://gw.example.com/proxy-ctx/mcp", envVarByKey(t, rec.bindingVars["default"], "GITHUB_MCP_URL").Value)
	require.ElementsMatch(t, rec.bindingVars["default"], rec.componentVars)
}

// Inject a transaction completion failure without a live DB. Repository behavior
// remains mocked; the service still executes GORM's real transaction callback and commit.
type mcpRenameCommitPool struct {
	gorm.ConnPool
	tx     *mcpRenameCommitTx
	begins int
}

func (p *mcpRenameCommitPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	p.begins++
	if p.begins > 1 {
		return &mcpRenameCommitTx{onCommit: func() {}}, nil
	}
	return p.tx, nil
}

type mcpRenameCommitTx struct {
	gorm.ConnPool
	commitErr error
	onCommit  func()
	commits   int
	rollbacks int
}

func (tx *mcpRenameCommitTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}

func (tx *mcpRenameCommitTx) Commit() error {
	tx.commits++
	tx.onCommit()
	return tx.commitErr
}

func (tx *mcpRenameCommitTx) Rollback() error {
	tx.rollbacks++
	return nil
}

func TestUpdateMCPConfigEnvironmentVariableNames_CommitFailureRestoresLiveValues(t *testing.T) {
	for _, scenario := range []string{"commit succeeds", "commit fails", "later rename commits", "other config claims target", "commit response lost"} {
		failCommit := scenario != "commit succeeds"
		t.Run(scenario, func(t *testing.T) {
			f := newMCPURLTestFixture()
			configUUID := uuid.New()
			rec := &mcpRenameRecorder{}
			tx := &mcpRenameCommitTx{onCommit: func() {
				require.Equal(t, "GITHUB_URL", rec.componentVars[0].Key, "live rewrite must complete before commit")
				if scenario == "other config claims target" {
					winner := []client.EnvVar{{Key: "GITHUB_URL", Value: "winning-other-config"}}
					rec.componentVars = mergeRenameTestVars(rec.componentVars, nil, winner)
					rec.bindingVars["default"] = mergeRenameTestVars(rec.bindingVars["default"], nil, winner)
				}
				if scenario == "later rename commits" {
					// Model the next request winning the row lock after this transaction ends.
					next := []client.EnvVar{{Key: "NEXT_URL", Value: "https://next/mcp"}, {Key: "NEXT_API_KEY", ValueFrom: &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "next-secret", Key: "api-key"}}}}
					rec.componentVars = append(rec.componentVars, next...)
					rec.bindingVars["default"] = append(rec.bindingVars["default"], next...)
				}
			}}
			if failCommit {
				tx.commitErr = errors.New("transaction aborted at commit")
			}
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: &mcpRenameCommitPool{tx: tx}}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			svc := f.newService()
			svc.db, svc.ocClient = database, rec.ocClient()
			removeKeys := func(vars []client.EnvVar, keys []string) []client.EnvVar {
				result := make([]client.EnvVar, 0, len(vars))
				for _, v := range vars {
					remove := false
					for _, key := range keys {
						if v.Key == key {
							remove = true
						}
					}
					if !remove {
						result = append(result, v)
					}
				}
				return result
			}
			oc := svc.ocClient.(*clientmocks.OpenChoreoClientMock)
			recordRemove := oc.RemoveComponentEnvironmentVariablesFunc
			oc.RemoveComponentEnvironmentVariablesFunc = func(ctx context.Context, org, project, agent string, keys []string) error {
				rec.componentVars = removeKeys(rec.componentVars, keys)
				return recordRemove(ctx, org, project, agent, keys)
			}
			oc.RemoveReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, env string, keys []string) error {
				rec.bindingVars[env] = removeKeys(rec.bindingVars[env], keys)
				return nil
			}

			// Model the client's actual remove-then-upsert behavior, retaining unrelated keys.
			oc.UpdateComponentEnvVarsFunc = func(_ context.Context, _, _, _ string, values []client.EnvVar) error {
				rec.componentVars = mergeRenameTestVars(rec.componentVars, nil, values)
				return nil
			}
			oc.ReplaceReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, env string, keys []string, values []client.EnvVar) error {
				rec.bindingRemoved[env] = keys
				rec.bindingVars[env] = mergeRenameTestVars(rec.bindingVars[env], keys, values)
				return nil
			}

			lockReads := 0
			svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
				ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
					return []models.EnvAgentMCPMapping{f.mapping}, nil
				},
			}
			svc.envVariableRepo = &repomocks.AgentEnvConfigVariableRepositoryMock{
				ListByConfigForUpdateFunc: func(context.Context, *gorm.DB, uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
					lockReads++
					rows := renameVarRows(configUUID, f.envUUID, "default-secrets")
					if lockReads > 1 && ((scenario == "later rename commits") || scenario == "commit response lost") {
						for i := range rows {
							if scenario == "commit response lost" {
								rows[i].VariableName = map[string]string{"url": "GITHUB_URL", "apikey": "GITHUB_API_KEY"}[rows[i].VariableKey]
							} else {
								rows[i].VariableName = map[string]string{"url": "NEXT_URL", "apikey": "NEXT_API_KEY"}[rows[i].VariableKey]
							}
						}
					}
					return rows, nil
				},
				UpdateVariableNamesFunc: func(_ context.Context, _ *gorm.DB, id uuid.UUID, names map[string]string) error {
					require.Equal(t, configUUID, id)
					require.Equal(t, map[string]string{"url": "GITHUB_URL", "apikey": "GITHUB_API_KEY"}, names)
					return nil
				},
			}
			variableRepo := svc.envVariableRepo.(*repomocks.AgentEnvConfigVariableRepositoryMock)
			variableRepo.LockAndListByAgentFunc = func(ctx context.Context, tx *gorm.DB, _, _, _ string) ([]models.AgentEnvConfigVariable, error) {
				rows, readErr := variableRepo.ListByConfigForUpdateFunc(ctx, tx, configUUID)
				if lockReads > 1 && scenario == "other config claims target" {
					rows = append(rows, models.AgentEnvConfigVariable{ConfigUUID: uuid.New(), VariableName: "GITHUB_URL"})
				}
				lockReads--
				return rows, readErr
			}
			config := &models.AgentConfiguration{UUID: configUUID, OUID: f.ouID, ProjectName: "default", AgentID: "agent", Name: "github"}
			err = svc.updateMCPConfigEnvironmentVariableNames(context.Background(), config, f.ouID, "default", "agent",
				map[string]string{f.envUUID.String(): "default"}, false, "default", []models.EnvironmentVariableConfig{
					{Key: "url", Name: "GITHUB_URL"}, {Key: "apikey", Name: "GITHUB_API_KEY"},
				})
			require.Equal(t, 1, tx.commits)
			if (scenario == "later rename commits") || scenario == "commit response lost" {
				require.ErrorIs(t, err, tx.commitErr)
				require.Equal(t, 2, lockReads)
				expected := "GITHUB_URL"
				if scenario == "later rename commits" {
					expected = "NEXT_URL"
				}
				require.Equal(t, expected, rec.componentVars[0].Key, "recovery must not restore stale names after revision advances")
				expectedCount := 2
				require.Len(t, rec.componentVars, expectedCount, "only unsaved orphaned keys must be removed")
				require.Len(t, rec.bindingVars["default"], expectedCount)
				if scenario == "later rename commits" {
					require.Equal(t, "https://next/mcp", envVarByKey(t, rec.componentVars, "NEXT_URL").Value)
					require.Equal(t, "next-secret", envVarByKey(t, rec.componentVars, "NEXT_API_KEY").ValueFrom.SecretKeyRef.Name)
				}
				require.Len(t, svc.envVariableRepo.(*repomocks.AgentEnvConfigVariableRepositoryMock).ListByConfigForUpdateCalls(), 2)
			} else if failCommit {
				require.ErrorIs(t, err, tx.commitErr)
				require.Equal(t, 1, tx.rollbacks)
				if scenario == "other config claims target" {
					require.Equal(t, []string{"GITHUB_API_KEY"}, rec.bindingRemoved["default"])
					require.Equal(t, "winning-other-config", envVarByKey(t, rec.componentVars, "GITHUB_URL").Value)
					require.Len(t, rec.componentVars, 3)
				} else {
					require.ElementsMatch(t, []string{"GITHUB_URL", "GITHUB_API_KEY"}, rec.bindingRemoved["default"])
				}
				require.Equal(t, "https://gw.example.com/proxy-ctx/mcp", envVarByKey(t, rec.componentVars, "GITHUB_MCP_URL").Value)
				require.Equal(t, "default-secrets", envVarByKey(t, rec.componentVars, "GITHUB_MCP_API_KEY").ValueFrom.SecretKeyRef.Name)
			} else {
				require.NoError(t, err)
				require.Zero(t, tx.rollbacks)
				require.ElementsMatch(t, []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"}, rec.bindingRemoved["default"])
			}
			require.ElementsMatch(t, rec.componentVars, rec.bindingVars["default"])
		})
	}
}

func TestApplyRenamedMCPEnvVars_MappedStageUsesOwnValues(t *testing.T) {
	f := newMCPURLTestFixture()
	stage := uuid.New()
	configID := uuid.New()
	stageProxy := *f.mapping.MCPProxy
	stageContext := "/stage-mcp"
	stageProxy.Configuration.Context = &stageContext
	stageProxy.Endpoints = []models.MCPProxyEndpoint{{Environments: []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: stage, ArtifactUUID: f.sharedArtifactUUID}}}}
	stageMapping := f.mapping
	stageMapping.EnvironmentUUID, stageMapping.MCPProxy = stage, &stageProxy
	f.gatewayRepo.EnvironmentMappingExistsFunc = func(_, env string) (bool, error) { return env == stage.String() || env == f.envUUID.String(), nil }
	svc := f.newService()
	rec := &mcpRenameRecorder{}
	svc.ocClient = rec.ocClient()
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
		return []models.EnvAgentMCPMapping{f.mapping, stageMapping}, nil
	}}
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: configID, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "default", stage.String(): "stage"}, "default", append(renameVarRows(configID, f.envUUID, "default-secret"), renameVarRows(configID, stage, "stage-secret")...), renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com/proxy-ctx/mcp", envVarByKey(t, rec.bindingVars["default"], "GITHUB_URL").Value)
	require.Equal(t, "https://gw.example.com/stage-mcp/mcp", envVarByKey(t, rec.bindingVars["stage"], "GITHUB_URL").Value)
	require.Equal(t, "default-secret", envVarByKey(t, rec.bindingVars["default"], "GITHUB_API_KEY").ValueFrom.SecretKeyRef.Name)
	require.Equal(t, client.SecretKeyRef{Name: "stage-secret", Key: "api-key"}, *envVarByKey(t, rec.bindingVars["stage"], "GITHUB_API_KEY").ValueFrom.SecretKeyRef)
}

func TestApplyRenamedMCPEnvVars_MissingFirstEnvironmentPreservesComponentValues(t *testing.T) {
	f := newMCPURLTestFixture()
	configID := uuid.New()
	svc := f.newService()
	rec := &mcpRenameRecorder{}
	oc := rec.ocClient()
	oldVars := []client.EnvVar{{Key: "GITHUB_MCP_URL", Value: "https://saved-base/mcp"}, {Key: "GITHUB_MCP_API_KEY", ValueFrom: &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "saved-secret", Key: "api-key"}}}}
	oc.GetComponentBaseEnvVarsFunc = func(context.Context, string, string) ([]client.EnvVar, error) {
		return append(append([]client.EnvVar{}, oldVars...), client.EnvVar{Key: "UNRELATED", Value: "keep"}), nil
	}
	svc.ocClient = oc
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
		return []models.EnvAgentMCPMapping{f.mapping}, nil
	}}
	undo, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: configID, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "stage"}, "default", renameVarRows(configID, f.envUUID, "stage-secret"), renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
	require.NoError(t, err)
	require.Len(t, rec.componentVars, 2)
	require.Equal(t, "https://saved-base/mcp", envVarByKey(t, rec.componentVars, "GITHUB_URL").Value)
	require.Equal(t, "saved-secret", envVarByKey(t, rec.componentVars, "GITHUB_API_KEY").ValueFrom.SecretKeyRef.Name)
	require.NoError(t, undo.undo())
	require.ElementsMatch(t, oldVars, rec.componentVars)
}

func TestApplyRenamedMCPEnvVars_PartialRenameFailureKeepsUnchangedComponentKey(t *testing.T) {
	f := newMCPURLTestFixture()
	id := uuid.New()
	svc := f.newService()
	values := map[string]client.EnvVar{
		"GITHUB_MCP_URL":     {Key: "GITHUB_MCP_URL", Value: "https://saved/mcp"},
		"GITHUB_MCP_API_KEY": {Key: "GITHUB_MCP_API_KEY", ValueFrom: &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "saved-secret", Key: "api-key"}}},
	}
	failure := errors.New("binding failed")
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return []models.EnvVars{}, nil
		},
		GetComponentBaseEnvVarsFunc: func(context.Context, string, string) ([]client.EnvVar, error) {
			return []client.EnvVar{values["GITHUB_MCP_URL"], values["GITHUB_MCP_API_KEY"]}, nil
		},
		RemoveComponentEnvironmentVariablesFunc: func(_ context.Context, _, _, _ string, keys []string) error {
			for _, k := range keys {
				delete(values, k)
			}
			return nil
		},
		UpdateComponentEnvVarsFunc: func(_ context.Context, _, _, _ string, vars []client.EnvVar) error {
			for _, v := range vars {
				values[v.Key] = v
			}
			return nil
		},
		ReplaceReleaseBindingEnvVarsFunc: func(context.Context, string, string, string, string, []string, []client.EnvVar) error { return failure },
	}
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
		return []models.EnvAgentMCPMapping{f.mapping}, nil
	}}
	templates := renamedTemplates()
	templates[1].Name = "GITHUB_MCP_API_KEY"
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: id, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "stage"}, "default", renameVarRows(id, f.envUUID, "stage-secret"), templates, []string{"GITHUB_MCP_URL"})
	require.ErrorIs(t, err, failure)
	require.Len(t, values, 2)
	require.Equal(t, "https://saved/mcp", values["GITHUB_MCP_URL"].Value)
	require.NotNil(t, values["GITHUB_MCP_API_KEY"].ValueFrom)
	require.Equal(t, "saved-secret", values["GITHUB_MCP_API_KEY"].ValueFrom.SecretKeyRef.Name)
}

func TestApplyRenamedMCPEnvVars_ProxyLookupFailureWritesNothing(t *testing.T) {
	f := newMCPURLTestFixture()
	f.mapping.MCPProxy.Endpoints = nil
	svc := f.newService()
	rec := &mcpRenameRecorder{}
	svc.ocClient = rec.ocClient()
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
		return []models.EnvAgentMCPMapping{f.mapping}, nil
	}}
	failure := errors.New("proxy database unavailable")
	svc.mcpProxyRepo = &repomocks.MCPProxyRepositoryMock{GetByUUIDFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return nil, failure }}
	id := uuid.New()
	_, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: id, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "default"}, "default", renameVarRows(id, f.envUUID, "secret"), renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
	require.ErrorIs(t, err, failure)
	require.Zero(t, rec.writes)
}

func TestApplyRenamedMCPEnvVars_RejectsExistingTargets(t *testing.T) {
	for _, location := range []string{"component", "binding"} {
		t.Run(location, func(t *testing.T) {
			f := newMCPURLTestFixture()
			svc := f.newService()
			rec := &mcpRenameRecorder{}
			oc := rec.ocClient()
			if location == "component" {
				oc.GetComponentBaseEnvVarsFunc = func(context.Context, string, string) ([]client.EnvVar, error) {
					return []client.EnvVar{{Key: "GITHUB_URL", Value: "agent-value"}}, nil
				}
			} else {
				oc.GetComponentConfigurationsFunc = func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
					return []models.EnvVars{{Key: "GITHUB_URL", IsSensitive: true, SecretRef: "agent-secret", SecretKey: "custom-property"}}, nil
				}
			}
			svc.ocClient = oc
			svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return []models.EnvAgentMCPMapping{f.mapping}, nil
			}}
			id := uuid.New()
			_, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: id, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "default"}, "default", renameVarRows(id, f.envUUID, "secret"), renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			require.Zero(t, rec.writes, "collision validation must precede all live writes")
		})
	}
}

func TestApplyRenamedMCPEnvVars_SnapshotFailureWritesNothing(t *testing.T) {
	for _, location := range []string{"component", "binding"} {
		t.Run(location, func(t *testing.T) {
			f := newMCPURLTestFixture()
			svc := f.newService()
			rec := &mcpRenameRecorder{}
			oc := rec.ocClient()
			failure := errors.New("snapshot unavailable")
			if location == "component" {
				oc.GetComponentBaseEnvVarsFunc = func(context.Context, string, string) ([]client.EnvVar, error) { return nil, failure }
			} else {
				oc.GetComponentConfigurationsFunc = func(context.Context, string, string, string, string) ([]models.EnvVars, error) { return nil, failure }
			}
			svc.ocClient = oc
			svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return []models.EnvAgentMCPMapping{f.mapping}, nil
			}}
			id := uuid.New()
			_, err := svc.applyRenamedMCPEnvVars(context.Background(), &models.AgentConfiguration{UUID: id, Name: "github"}, f.ouID, "default", "agent", map[string]string{f.envUUID.String(): "default"}, "default", renameVarRows(id, f.envUUID, "secret"), renamedTemplates(), []string{"GITHUB_MCP_URL", "GITHUB_MCP_API_KEY"})
			require.ErrorIs(t, err, failure)
			require.Zero(t, rec.writes)
		})
	}
}

func TestRenamedMCPProxyURL_ConfirmedMissingEndpointIsEmpty(t *testing.T) {
	f := newMCPURLTestFixture()
	f.mapping.MCPProxy.Endpoints = nil
	svc := f.newService()
	svc.mcpProxyRepo = &repomocks.MCPProxyRepositoryMock{GetByUUIDFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return f.mapping.MCPProxy, nil }}
	url, err := svc.renamedMCPProxyURL(context.Background(), &models.AgentConfiguration{Name: "github"}, &f.mapping, "default", f.ouID)
	require.NoError(t, err)
	require.Empty(t, url)
}

// mergeRenameTestVars mirrors OpenChoreo's key-based merge rather than replacing
// the whole slice, so unrelated and orphaned values remain visible to assertions.
func mergeRenameTestVars(existing []client.EnvVar, removed []string, values []client.EnvVar) []client.EnvVar {
	keys := make(map[string]bool)
	for _, key := range removed {
		keys[key] = true
	}
	for _, v := range values {
		keys[v.Key] = true
	}
	result := make([]client.EnvVar, 0, len(existing)+len(values))
	for _, v := range existing {
		if !keys[v.Key] {
			result = append(result, v)
		}
	}
	return append(result, values...)
}

func TestUpdateMCPConfigEnvironmentVariableNames_RejectsOtherConfigOwnership(t *testing.T) {
	for _, scenario := range []string{"collision", "ownership read fails", "own names allowed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMCPURLTestFixture()
			configID := uuid.New()
			tx := &mcpRenameCommitTx{onCommit: func() {}}
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: &mcpRenameCommitPool{tx: tx}}), &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			svc := f.newService()
			svc.db = database
			readErr := errors.New("ownership read failed")
			locked := false
			repo := &repomocks.AgentEnvConfigVariableRepositoryMock{
				LockAndListByAgentFunc: func(_ context.Context, _ *gorm.DB, org, project, agent string) ([]models.AgentEnvConfigVariable, error) {
					require.Equal(t, []string{f.ouID, "project", "agent"}, []string{org, project, agent})
					locked = true
					if scenario == "ownership read fails" {
						return nil, readErr
					}
					owner := uuid.New()
					if scenario == "own names allowed" {
						owner = configID
					}
					return []models.AgentEnvConfigVariable{{ConfigUUID: owner, VariableName: "GITHUB_URL"}}, nil
				},
				ListByConfigForUpdateFunc: func(context.Context, *gorm.DB, uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
					require.True(t, locked, "agent ownership lock must precede row locks")
					return renameVarRows(configID, f.envUUID, "secret"), nil
				},
			}
			svc.envVariableRepo = repo
			if scenario == "own names allowed" {
				repo.UpdateVariableNamesFunc = func(context.Context, *gorm.DB, uuid.UUID, map[string]string) error { return nil }
			}
			// No client methods are configured: rejected requests must never write live values.
			svc.ocClient = &clientmocks.OpenChoreoClientMock{}
			err = svc.updateMCPConfigEnvironmentVariableNames(context.Background(), &models.AgentConfiguration{UUID: configID, Name: "github"}, f.ouID, "project", "agent", nil, scenario == "own names allowed", "default", []models.EnvironmentVariableConfig{{Key: "url", Name: "GITHUB_URL"}})
			switch scenario {
			case "collision":
				require.ErrorIs(t, err, utils.ErrInvalidInput)
			case "ownership read fails":
				require.ErrorIs(t, err, readErr)
				require.NotErrorIs(t, err, utils.ErrInvalidInput)
			default:
				require.NoError(t, err)
			}
			if scenario != "own names allowed" {
				require.Empty(t, repo.UpdateVariableNamesCalls())
				require.Zero(t, tx.commits)
			}
		})
	}
}
