//
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

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// warmPoolTree builds a resource tree containing a SandboxWarmPool node with the given
// status, mirroring the shape returned by the OpenChoreo k8sresources/tree endpoint.
func warmPoolTree(status map[string]interface{}) *gen.K8sResourceTreeResponse {
	pool := gen.ResourceNode{
		Kind: resourceKindSandboxWarmPool,
		Name: "it-helpdesk-agent",
		Uid:  "pool-uid",
		Object: map[string]interface{}{
			"kind": resourceKindSandboxWarmPool,
		},
	}
	if status != nil {
		pool.Object["status"] = status
	}

	return &gen.K8sResourceTreeResponse{
		RenderedReleases: []gen.ReleaseResourceTree{{
			Name: "it-helpdesk-agent-default",
			Nodes: []gen.ResourceNode{
				{Kind: "Service", Name: "it-helpdesk-agent", Uid: "svc-uid", Object: map[string]interface{}{}},
				pool,
			},
		}},
	}
}

// externalSecretNode builds an ExternalSecret node whose own status reports the given Ready
// condition. Its health field is deliberately Healthy: that is what OpenChoreo reports even
// for a secret that is failing to sync, which is why the object status is what gets read.
func externalSecretNode(name, condStatus, reason, message string) gen.ResourceNode {
	healthy := gen.HealthInfo{}
	return gen.ResourceNode{
		Kind:   "ExternalSecret",
		Name:   name,
		Uid:    "es-" + name,
		Health: &healthy,
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type": "Ready", "status": condStatus, "reason": reason, "message": message,
					},
				},
			},
		},
	}
}

// treeWith builds a tree from a warm pool status plus any extra nodes.
func treeWith(poolStatus map[string]interface{}, extra ...gen.ResourceNode) *gen.K8sResourceTreeResponse {
	tree := warmPoolTree(poolStatus)
	tree.RenderedReleases[0].Nodes = append(tree.RenderedReleases[0].Nodes, extra...)
	return tree
}

func readyBinding() *gen.ReleaseBinding {
	return &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "it-helpdesk-agent-default"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "default"},
		Status: &gen.ReleaseBindingStatus{
			Conditions: &[]gen.Condition{{Type: "Ready", Status: "True", Reason: "Ready"}},
		},
	}
}

func TestRuntimeReplicaStateFromTree(t *testing.T) {
	t.Run("nil tree is unknown", func(t *testing.T) {
		assert.Equal(t, runtimeReplicaState{}, runtimeReplicaStateFromTree(nil))
	})

	t.Run("no warm pool node is unknown", func(t *testing.T) {
		tree := &gen.K8sResourceTreeResponse{
			RenderedReleases: []gen.ReleaseResourceTree{{
				Name:  "r",
				Nodes: []gen.ResourceNode{{Kind: "Service", Name: "svc", Uid: "u", Object: map[string]interface{}{}}},
			}},
		}
		assert.False(t, runtimeReplicaStateFromTree(tree).found)
	})

	t.Run("all replicas ready", func(t *testing.T) {
		// JSON numbers decode as float64, which is how the counts actually arrive.
		got := runtimeReplicaStateFromTree(warmPoolTree(map[string]interface{}{
			"replicas": float64(1), "readyReplicas": float64(1),
		}))
		assert.Equal(t, runtimeReplicaState{found: true, desired: 1, ready: 1}, got)
		assert.False(t, got.isBooting())
	})

	t.Run("no replica ready yet", func(t *testing.T) {
		// The warm pool omits readyReplicas entirely while the pod boots.
		got := runtimeReplicaStateFromTree(warmPoolTree(map[string]interface{}{"replicas": float64(1)}))
		assert.Equal(t, runtimeReplicaState{found: true, desired: 1, ready: 0}, got)
		assert.True(t, got.isBooting())
	})

	t.Run("pool without status is not ready", func(t *testing.T) {
		got := runtimeReplicaStateFromTree(warmPoolTree(nil))
		assert.True(t, got.found)
		assert.False(t, got.isBooting(), "desired is unknown, so nothing is claimed about readiness")
	})

	t.Run("scaled to zero is not booting", func(t *testing.T) {
		got := runtimeReplicaStateFromTree(warmPoolTree(map[string]interface{}{
			"replicas": float64(0), "readyReplicas": float64(0),
		}))
		assert.False(t, got.isBooting(), "an agent scaled to zero is not mid-startup")
	})

	t.Run("partially ready pool counts as ready", func(t *testing.T) {
		got := runtimeReplicaStateFromTree(warmPoolTree(map[string]interface{}{
			"replicas": float64(3), "readyReplicas": float64(1),
		}))
		assert.False(t, got.isBooting(), "one ready replica can already serve traffic")
	})
}

func TestDetermineDeploymentStatusWithRuntimeReadiness(t *testing.T) {
	t.Run("ready binding with no ready replica is in progress", func(t *testing.T) {
		booting := runtimeReplicaState{found: true, desired: 1, ready: 0}
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(readyBinding(), booting))
	})

	t.Run("ready binding with a ready replica is active", func(t *testing.T) {
		serving := runtimeReplicaState{found: true, desired: 1, ready: 1}
		assert.Equal(t, DeploymentStatusActive, determineDeploymentStatus(readyBinding(), serving))
	})

	t.Run("unknown readiness falls back to the binding", func(t *testing.T) {
		assert.Equal(t, DeploymentStatusActive, determineDeploymentStatus(readyBinding(), runtimeReplicaState{}))
	})

	t.Run("readiness does not override suspended", func(t *testing.T) {
		binding := readyBinding()
		state := gen.ReleaseBindingSpecStateUndeploy
		binding.Spec.State = &state
		booting := runtimeReplicaState{found: true, desired: 1, ready: 0}
		assert.Equal(t, DeploymentStatusSuspended, determineDeploymentStatus(binding, booting))
	})

	t.Run("readiness does not override failed", func(t *testing.T) {
		binding := readyBinding()
		binding.Status.Conditions = &[]gen.Condition{{Type: "Ready", Status: "False", Reason: "Failed"}}
		booting := runtimeReplicaState{found: true, desired: 1, ready: 0}
		assert.Equal(t, DeploymentStatusFailed, determineDeploymentStatus(binding, booting))
	})

	t.Run("nil binding is not deployed", func(t *testing.T) {
		assert.Equal(t, DeploymentStatusNotDeployed, determineDeploymentStatus(nil, runtimeReplicaState{}))
	})
}

// A resource reporting itself not ready, with nothing serving, must report failed rather
// than sitting at in-progress forever. Reproduces the observed case: an ExternalSecret that
// will not sync leaves the container in CreateContainerConfigError ("secret ... not found")
// indefinitely.
func TestNotReadyResourceIsReportedAsFailed(t *testing.T) {
	const syncErr = "could not get secret data from provider"

	t.Run("unsyncable secret with nothing ready is failed", func(t *testing.T) {
		tree := treeWith(
			map[string]interface{}{"replicas": float64(1)},
			externalSecretNode("env-secrets", "False", "SecretSyncedError", syncErr),
		)
		state := runtimeReplicaStateFromTree(tree)

		assert.True(t, state.isFailed())
		assert.False(t, state.isBooting(), "a pod that cannot start is not starting")
		assert.Contains(t, state.notReadyResource, "SecretSyncedError")
		assert.Equal(t, DeploymentStatusFailed, determineDeploymentStatus(readyBinding(), state))
	})

	t.Run("unsyncable secret while a replica still serves stays active", func(t *testing.T) {
		// Observed live: the ExternalSecret was failing for ~30 minutes while the pod ran on
		// a previously synced Secret. The agent was serving, so it is not failed.
		tree := treeWith(
			map[string]interface{}{"replicas": float64(1), "readyReplicas": float64(1)},
			externalSecretNode("env-secrets", "False", "SecretSyncedError", syncErr),
		)
		state := runtimeReplicaStateFromTree(tree)

		assert.False(t, state.isFailed())
		assert.Equal(t, DeploymentStatusActive, determineDeploymentStatus(readyBinding(), state))
	})

	t.Run("synced secrets with nothing ready is still in progress", func(t *testing.T) {
		tree := treeWith(
			map[string]interface{}{"replicas": float64(1)},
			externalSecretNode("env-secrets", "True", "SecretSynced", "secret synced"),
		)
		state := runtimeReplicaStateFromTree(tree)

		assert.Empty(t, state.notReadyResource)
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(readyBinding(), state))
	})

	t.Run("secret with no conditions is not treated as a failure", func(t *testing.T) {
		node := gen.ResourceNode{
			Kind: "ExternalSecret", Name: "fresh", Uid: "u",
			Object: map[string]interface{}{},
		}
		state := runtimeReplicaStateFromTree(treeWith(map[string]interface{}{"replicas": float64(1)}, node))

		assert.Empty(t, state.notReadyResource)
		assert.True(t, state.isBooting())
	})

	t.Run("any kind reporting Ready=False counts, not just ExternalSecret", func(t *testing.T) {
		// No kind is special-cased. ExternalSecret is merely the only kind in the tree today
		// that publishes a Ready condition; Backend and RestApi use Accepted/Programmed and
		// so are ignored whatever they report.
		blocked := externalSecretNode("x", "False", "Blocked", "nope")
		blocked.Kind = "SomeFutureKind"
		accepted := externalSecretNode("gw", "False", "NotAccepted", "ignored")
		accepted.Kind = "Backend"
		accepted.Object["status"].(map[string]interface{})["conditions"] = []interface{}{
			map[string]interface{}{"type": "Accepted", "status": "False", "reason": "NotAccepted"},
		}

		state := runtimeReplicaStateFromTree(
			treeWith(map[string]interface{}{"replicas": float64(1)}, accepted, blocked),
		)

		assert.Contains(t, state.notReadyResource, "SomeFutureKind")
		assert.NotContains(t, state.notReadyResource, "Backend", "a non-Ready condition type is ignored")
	})

	t.Run("scaled to zero with a failing secret is not failed", func(t *testing.T) {
		tree := treeWith(
			map[string]interface{}{"replicas": float64(0), "readyReplicas": float64(0)},
			externalSecretNode("env-secrets", "False", "SecretSyncedError", syncErr),
		)
		assert.False(t, runtimeReplicaStateFromTree(tree).isFailed())
	})
}

// bindingBootingSince returns a Ready binding whose last deploy happened `ago` in the past,
// which is what bootDeadlineExceeded measures against. LastSpecUpdateTime is set because
// getLastDeployedTime takes the max of it and the Ready condition's transition time.
func bindingBootingSince(ago time.Duration) *gen.ReleaseBinding {
	binding := readyBinding()
	deployedAt := time.Now().Add(-ago)
	(*binding.Status.Conditions)[0].LastTransitionTime = deployedAt
	binding.Status.LastSpecUpdateTime = &deployedAt
	return binding
}

// An agent whose container never binds its port fails its TCP startup probe forever: the
// pod is replaced every few minutes, and because pods are absent from the resource tree and
// the warm pool reports Healthy with zero ready replicas, every payload the control plane
// sees is identical to a healthy boot. Without a deadline the deployment reports
// "in-progress" for as long as the agent exists.
func TestBootingPastTheStartupBudgetIsReportedAsFailed(t *testing.T) {
	booting := runtimeReplicaState{found: true, desired: 1, ready: 0}

	t.Run("within the budget is still in progress", func(t *testing.T) {
		binding := bindingBootingSince(agentStartupBudget / 2)
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("past the budget is failed", func(t *testing.T) {
		binding := bindingBootingSince(agentStartupBudget + time.Minute)
		assert.Equal(t, DeploymentStatusFailed, determineDeploymentStatus(binding, booting),
			"a startup probe that has already given up is not still starting")
	})

	t.Run("a serving agent is never failed by age", func(t *testing.T) {
		// The budget only applies to a boot in progress. A long-running healthy agent has
		// a last-deployed time far in the past and must stay active.
		serving := runtimeReplicaState{found: true, desired: 1, ready: 1}
		binding := bindingBootingSince(30 * 24 * time.Hour)
		assert.Equal(t, DeploymentStatusActive, determineDeploymentStatus(binding, serving))
	})

	t.Run("unknown readiness is never failed by age", func(t *testing.T) {
		// Tree fetch failed or no warm pool node: isBooting() is false, so the binding
		// alone decides and the deadline is never consulted.
		binding := bindingBootingSince(agentStartupBudget + time.Minute)
		assert.Equal(t, DeploymentStatusActive, determineDeploymentStatus(binding, runtimeReplicaState{}))
	})

	t.Run("a binding with no timestamps stays in progress", func(t *testing.T) {
		// getLastDeployedTime returns the zero time when the binding carries no condition
		// transition, no spec update and no creation timestamp. time.Since(zero) is
		// centuries, so without the IsZero guard every such binding would report failed.
		binding := readyBinding()
		assert.True(t, getLastDeployedTime(binding).IsZero(), "guards the premise of this case")
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("an old binding with no deploy timestamps stays in progress", func(t *testing.T) {
		// Creation dates the binding, not an attempt to start the agent. A binding that
		// has existed for weeks and is redeploying now must not be failed on the strength
		// of its age, so the budget is measured from deployAttemptTime, which excludes
		// the creation timestamp getLastDeployedTime falls back to.
		binding := readyBinding()
		created := time.Now().Add(-30 * 24 * time.Hour)
		binding.Metadata.CreationTimestamp = &created
		(*binding.Status.Conditions)[0].LastTransitionTime = time.Time{}
		binding.Status.LastSpecUpdateTime = nil

		assert.Equal(t, created, getLastDeployedTime(binding),
			"display still falls back to creation")
		assert.True(t, deployAttemptTime(binding).IsZero(),
			"but no deploy attempt is recorded")
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("an overrunning boot does not override suspended", func(t *testing.T) {
		binding := bindingBootingSince(agentStartupBudget + time.Minute)
		state := gen.ReleaseBindingSpecStateUndeploy
		binding.Spec.State = &state
		assert.Equal(t, DeploymentStatusSuspended, determineDeploymentStatus(binding, booting))
	})
}

// bindingWithStartupCheck is a binding booting since ago whose release runs the given
// startup check, written onto the binding the way a deploy writes it.
func bindingWithStartupCheck(ago time.Duration, startup map[string]interface{}) *gen.ReleaseBinding {
	binding := bindingBootingSince(ago)
	configs := map[string]interface{}{
		"healthChecks": map[string]interface{}{"startup": startup},
	}
	binding.Spec.ComponentTypeEnvironmentConfigs = &configs
	return binding
}

// bindingWithHealthChecks is a binding booting since ago whose release runs the given
// health checks, written onto the binding the way a deploy writes them.
func bindingWithHealthChecks(ago time.Duration, healthChecks map[string]interface{}) *gen.ReleaseBinding {
	binding := bindingBootingSince(ago)
	configs := map[string]interface{}{"healthChecks": healthChecks}
	binding.Spec.ComponentTypeEnvironmentConfigs = &configs
	return binding
}

// Kubelet runs no readiness check before its initial delay, counted from container start,
// so an agent that loads a model behind a long readiness delay cannot be ready sooner. The
// deploy must wait for it, startup check on or off.
func TestBootBudgetWaitsForTheFirstReadinessCheck(t *testing.T) {
	booting := runtimeReplicaState{found: true, desired: 1, ready: 0}

	// Startup check off, readiness delay 1200s: first readiness check by 1200s + 5s;
	// + 5m margin = 1505s.
	startupOffLongReadiness := map[string]interface{}{
		"startup":   map[string]interface{}{"enabled": false},
		"readiness": map[string]interface{}{"initialDelaySeconds": 1200},
	}

	t.Run("no readiness check uses the default probe's first check", func(t *testing.T) {
		assert.Equal(t, 5*time.Second, firstReadinessCheck(nil), "0s delay + 5s interval")
	})

	t.Run("a switched-off readiness check adds nothing", func(t *testing.T) {
		off := false
		assert.Equal(t, time.Duration(0), firstReadinessCheck(&models.HealthCheck{Enabled: &off}))
	})

	t.Run("startup off with a long readiness delay is still in progress past 10 minutes", func(t *testing.T) {
		binding := bindingWithHealthChecks(15*time.Minute, startupOffLongReadiness)
		assert.Equal(t, 1205*time.Second+agentPreStartMargin, bootBudget(binding))
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("startup off with a long readiness delay is failed past its own budget", func(t *testing.T) {
		binding := bindingWithHealthChecks(26*time.Minute, startupOffLongReadiness)
		assert.Equal(t, DeploymentStatusFailed, determineDeploymentStatus(binding, booting))
	})

	t.Run("a readiness delay longer than the default startup window wins", func(t *testing.T) {
		// Default startup check (310s), readiness delay 1200s + interval 10s.
		binding := bindingWithHealthChecks(15*time.Minute, map[string]interface{}{
			"readiness": map[string]interface{}{"initialDelaySeconds": 1200, "periodSeconds": 10},
		})
		assert.Equal(t, 1210*time.Second+agentPreStartMargin, bootBudget(binding))
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("a startup window longer than the readiness delay wins", func(t *testing.T) {
		binding := bindingWithHealthChecks(0, map[string]interface{}{
			"startup":   map[string]interface{}{"periodSeconds": 10, "failureThreshold": 120},
			"readiness": map[string]interface{}{"initialDelaySeconds": 600},
		})
		assert.Equal(t, 1210*time.Second+agentPreStartMargin, bootBudget(binding))
	})
}

// An agent may set a startup check far longer than the default (up to an hour). The deploy
// must wait that long before calling it failed, or the page shows Failed for an agent that
// then becomes ready and serves traffic.
func TestBootBudgetFollowsTheStartupCheck(t *testing.T) {
	booting := runtimeReplicaState{found: true, desired: 1, ready: 0}

	// 10s + 10s x 120 = 1210s; + 5m margin = 1510s.
	slowStartup := map[string]interface{}{
		"initialDelaySeconds": 10,
		"periodSeconds":       10,
		"failureThreshold":    120,
	}

	t.Run("no startup check uses the default probe's window", func(t *testing.T) {
		// 10s + 5s x 60 = 310s; + 5m margin = 10m10s, just over the floor.
		assert.Equal(t, 310*time.Second, startupProbeWindow(bindingHealthChecks(readyBinding()).Startup))
		assert.Equal(t, 310*time.Second+agentPreStartMargin, bootBudget(readyBinding()))
	})

	t.Run("a long startup check raises the budget", func(t *testing.T) {
		binding := bindingWithStartupCheck(0, slowStartup)
		assert.Equal(t, 1210*time.Second, startupProbeWindow(bindingHealthChecks(binding).Startup))
		assert.Equal(t, 1210*time.Second+agentPreStartMargin, bootBudget(binding))
	})

	t.Run("a long startup check is still in progress past 10 minutes", func(t *testing.T) {
		binding := bindingWithStartupCheck(15*time.Minute, slowStartup)
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("a long startup check is failed past its own budget", func(t *testing.T) {
		binding := bindingWithStartupCheck(26*time.Minute, slowStartup)
		assert.Equal(t, DeploymentStatusFailed, determineDeploymentStatus(binding, booting))
	})

	t.Run("a short startup check never goes below the floor", func(t *testing.T) {
		binding := bindingWithStartupCheck(0, map[string]interface{}{"failureThreshold": 3})
		assert.Equal(t, 25*time.Second, startupProbeWindow(bindingHealthChecks(binding).Startup), "10s + 5s x 3, the rest defaulted")
		assert.Equal(t, agentStartupBudget, bootBudget(binding))
	})

	t.Run("a switched-off startup check uses the floor", func(t *testing.T) {
		binding := bindingWithStartupCheck(0, map[string]interface{}{"enabled": false, "failureThreshold": 300})
		assert.Equal(t, time.Duration(0), startupProbeWindow(bindingHealthChecks(binding).Startup))
		assert.Equal(t, agentStartupBudget, bootBudget(binding))
	})

	t.Run("a timeout longer than the interval sets the gap between attempts", func(t *testing.T) {
		// Validation rejects this today; a binding saved before that rule may still carry it.
		binding := bindingWithStartupCheck(0, map[string]interface{}{
			"initialDelaySeconds": 0, "periodSeconds": 5, "timeoutSeconds": 20, "failureThreshold": 60,
		})
		assert.Equal(t, 1200*time.Second, startupProbeWindow(bindingHealthChecks(binding).Startup))
	})

	t.Run("a window too long for time.Duration does not wrap around", func(t *testing.T) {
		// Validation caps the window at an hour; a binding edited in OpenChoreo directly can carry more.
		// 100000s x 100000 = 1e10s, past the ~9.2e9s a time.Duration holds.
		binding := bindingWithStartupCheck(15*time.Minute, map[string]interface{}{
			"periodSeconds": 100000, "timeoutSeconds": 1, "failureThreshold": 100000,
		})
		assert.Greater(t, startupProbeWindow(bindingHealthChecks(binding).Startup), 24*time.Hour)
		assert.Greater(t, bootBudget(binding), startupProbeWindow(bindingHealthChecks(binding).Startup), "the margin still adds without overflowing")
		assert.Equal(t, DeploymentStatusInProgress, determineDeploymentStatus(binding, booting))
	})

	t.Run("a negative window counts as zero", func(t *testing.T) {
		binding := bindingWithStartupCheck(0, map[string]interface{}{"initialDelaySeconds": -100000})
		assert.Equal(t, time.Duration(0), startupProbeWindow(bindingHealthChecks(binding).Startup))
		assert.Equal(t, agentStartupBudget, bootBudget(binding))
	})

	t.Run("an unreadable startup check uses the default window", func(t *testing.T) {
		binding := bindingBootingSince(0)
		configs := map[string]interface{}{"healthChecks": "not an object"}
		binding.Spec.ComponentTypeEnvironmentConfigs = &configs
		assert.Equal(t, 310*time.Second, startupProbeWindow(bindingHealthChecks(binding).Startup))
	})
}

// The probed port is the one fact in the failure log that distinguishes a misdeclared port
// from an agent that is merely slow, so it must survive the shapes a binding can take.
func TestProbedPorts(t *testing.T) {
	t.Run("reads the service URL port", func(t *testing.T) {
		port := int32(8000)
		binding := readyBinding()
		binding.Status.Endpoints = &[]gen.EndpointURLStatus{{
			Name:       "it-helpdesk-endpoint",
			ServiceURL: &gen.EndpointURL{Host: "it-helpdesk.dp-default", Port: &port},
		}}
		assert.Equal(t, []int32{8000}, probedPorts(binding))
	})

	t.Run("an endpoint without a service URL contributes nothing", func(t *testing.T) {
		binding := readyBinding()
		binding.Status.Endpoints = &[]gen.EndpointURLStatus{{Name: "no-url"}}
		assert.Empty(t, probedPorts(binding))
	})

	t.Run("no endpoints is empty, not a panic", func(t *testing.T) {
		assert.Empty(t, probedPorts(readyBinding()))
	})
}
