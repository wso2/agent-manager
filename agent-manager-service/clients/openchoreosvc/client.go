// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package openchoreosvc

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// KubernetesConfigData holds the Kubernetes cluster configuration data
type KubernetesConfigData struct {
	ClusterName string
	CACert      string
	ClientCert  string
	ClientKey   string
}

// OpenChoreoSvcClient handles interactions with the OpenChoreo service

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/openchoreo_client_fake.go . OpenChoreoSvcClient:OpenChoreoSvcClientMock

type OpenChoreoSvcClient interface {
	CreateAgentComponent(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error
	AttachComponentTrait(ctx context.Context, orgName string, projName string, agentName string) error
	TriggerBuild(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error)
	GetProject(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error)
	ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error)
	ListProjects(ctx context.Context, orgName string) ([]*models.ProjectResponse, error)
	GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error)
	GetDeploymentPipelinesForOrganization(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error)
	DeleteProject(ctx context.Context, orgName string, projectName string) error
	GetDeploymentPipeline(ctx context.Context, orgName string, deploymentPipelineName string) (*models.DeploymentPipelineResponse, error)
	CreateProject(ctx context.Context, orgName string, projectName string, deploymentPipelineRef string, projectDisplayName string, projectDescription string) error
	GetAgentComponent(ctx context.Context, orgName string, projName string, agentName string) (*AgentComponent, error)
	ListAgentComponents(ctx context.Context, orgName string, projName string) ([]*AgentComponent, error)
	DeleteAgentComponent(ctx context.Context, orgName string, projName string, agentName string) error
	DeployAgentComponent(ctx context.Context, orgName string, projName string, componentName string, req *spec.DeployAgentRequest) error
	ListComponentWorkflows(ctx context.Context, orgName string, projName string, componentName string) ([]*models.BuildResponse, error)
	GetComponentWorkflow(ctx context.Context, orgName string, projName string, componentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, orgName string, pipelineName string, projName string, componentName string) ([]*models.DeploymentResponse, error)
	GetEnvironment(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error)
	IsAgentComponentExists(ctx context.Context, orgName string, projName string, agentName string) (bool, error)
	GetAgentEndpoints(ctx context.Context, orgName string, projName string, agentName string, environment string) (map[string]models.EndpointsResponse, error)
	GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	GetDataplanesForOrganization(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error)
}

type openChoreoSvcClient struct {
	client client.Client
}

// NewOpenChoreoSvcClient creates a new OpenChoreo service client instance
func NewOpenChoreoSvcClient() (OpenChoreoSvcClient, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Create a scheme and register the OpenChoreo v1alpha1 types
	sch := runtime.NewScheme()
	if err := scheme.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}
	if err := v1alpha1.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("failed to add v1alpha1 types to scheme: %w", err)
	}

	k8sClient, err := client.New(config, client.Options{
		Scheme: sch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &openChoreoSvcClient{
		client: k8sClient,
	}, nil
}

// getKubernetesConfig returns the Kubernetes configuration
func getKubernetesConfig() (*rest.Config, error) {
	if config.GetConfig().IsLocalDevEnv {
		kubeconfigPath := config.GetConfig().KubeConfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %w", err)
		}
		return config, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}
	return config, nil
}

func (k *openChoreoSvcClient) GetProject(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
	project := &v1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetProject", func() error {
		return k.client.Get(ctx, key, project)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return &models.ProjectResponse{
		UUID:               string(project.UID),
		Name:               project.Name,
		OrgName:            project.Namespace,
		DisplayName:        project.Annotations[string(AnnotationKeyDisplayName)],
		Description:        project.Annotations[string(AnnotationKeyDescription)],
		CreatedAt:          project.CreationTimestamp.Time,
		DeploymentPipeline: project.Spec.DeploymentPipelineRef,
	}, nil
}

func (k *openChoreoSvcClient) ListAgentComponents(ctx context.Context, orgName string, projName string) ([]*AgentComponent, error) {
	componentList := &v1alpha1.ComponentList{}
	err := k.retryK8sOperation(ctx, "ListComponents", func() error {
		return k.client.List(ctx, componentList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}
	var agentComponents []*AgentComponent
	for i := range componentList.Items {
		component := &componentList.Items[i]
		if component.Spec.Owner.ProjectName == projName {
			agentComponents = append(agentComponents, toComponentResponse(component))
		}
	}
	// Sort components by creation time descending
	sort.SliceStable(agentComponents, func(i, j int) bool {
		return agentComponents[i].CreatedAt.After(agentComponents[j].CreatedAt)
	})
	return agentComponents, nil
}

func (k *openChoreoSvcClient) IsAgentComponentExists(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
	component := &v1alpha1.Component{}
	key := client.ObjectKey{
		Name:      agentName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetComponent", func() error {
		return k.client.Get(ctx, key, component)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("failed to check component existence: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projName {
		return false, nil
	}

	return true, nil
}

func (k *openChoreoSvcClient) GetAgentComponent(ctx context.Context, orgName string, projName string, agentName string) (*AgentComponent, error) {
	component := &v1alpha1.Component{}
	key := client.ObjectKey{
		Name:      agentName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetComponent", func() error {
		return k.client.Get(ctx, key, component)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get agent component: %w", err)
	}
	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projName {
		return nil, fmt.Errorf("component does not belong to the specified project")
	}
	return toComponentResponse(component), nil
}

func (k *openChoreoSvcClient) AttachComponentTrait(ctx context.Context, orgName string, projName string, agentName string) error {
	openChoreoProject, err := k.GetProject(ctx, projName, orgName)
	if err != nil {
		return fmt.Errorf("failed to get project for trait attachment: %w", err)
	}
	pipelineName := openChoreoProject.DeploymentPipeline
	if pipelineName == "" {
		return fmt.Errorf("failed to attach trait: project %s does not have a deployment pipeline configured", projName)
	}
	pipeline, err := k.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		return fmt.Errorf("failed to get deployment pipeline for trait attachment: %w", err)
	}
	lowestEnvName := findLowestEnvironment(pipeline.PromotionPaths)
	openChoreoEnv, err := k.GetEnvironment(ctx, orgName, lowestEnvName)
	if err != nil {
		return fmt.Errorf("failed to get environment for trait attachment: %w", err)
	}
	component := &v1alpha1.Component{}
	key := client.ObjectKey{
		Name:      agentName,
		Namespace: orgName,
	}
	err = k.retryK8sOperation(ctx, "GetComponentForTraitAttachment", func() error {
		return k.client.Get(ctx, key, component)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return utils.ErrAgentNotFound
		}
		return fmt.Errorf("failed to get component for trait attachment: %w", err)
	}
	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projName {
		return fmt.Errorf("component does not belong to the specified project")
	}
	otelInstrumentationTrait, err := createOTELInstrumentationTrait(component, openChoreoEnv.UUID, openChoreoProject.UUID)
	if err != nil {
		return fmt.Errorf("error creating OTEL instrumentation trait: %w", err)
	}
	component.Spec.Traits = append(component.Spec.Traits, *otelInstrumentationTrait)
	err = k.retryK8sOperation(ctx, "UpdateComponentWithTrait", func() error {
		return k.client.Update(ctx, component)
	})
	if err != nil {
		return fmt.Errorf("failed to update component with trait: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) CreateAgentComponent(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error {
	var componentCR *v1alpha1.Component
	var err error

	if req.Provisioning.Type == string(utils.ExternalAgent) {
		componentCR, err = createComponentCRForExternalAgents(orgName, projName, req)
		if err != nil {
			return fmt.Errorf("failed to create component CR for external agents: %w", err)
		}
		err = k.retryK8sOperation(ctx, "CreateComponent", func() error {
			return k.client.Create(ctx, componentCR)
		})
		if err != nil {
			return fmt.Errorf("failed to create component: %w", err)
		}
	} else {
		componentCR, err = createComponentCRForInternalAgents(orgName, projName, req)
		if err != nil {
			return fmt.Errorf("failed to create component CR for internal agents: %w", err)
		}
		err = k.retryK8sOperation(ctx, "CreateComponent", func() error {
			return k.client.Create(ctx, componentCR)
		})
		if err != nil {
			return fmt.Errorf("failed to create component: %w", err)
		}
		// Add OpenTelemetry instrumentation trait for Python agents
		if req.AgentType.Type == string(utils.AgentTypeAPI) && req.RuntimeConfigs.Language == string(utils.LanguagePython) {
			err := k.AttachComponentTrait(ctx, orgName, projName, req.Name)
			if err != nil {
				return fmt.Errorf("error attaching OTEL instrumentation trait: %w", err)
			}
		}
	}
	
	return nil
}

func (k *openChoreoSvcClient) DeleteAgentComponent(ctx context.Context, orgName string, projName string, agentName string) error {
	component := &v1alpha1.Component{}
	key := client.ObjectKey{
		Name:      agentName,
		Namespace: orgName,
	}

	// First, get the component to verify it exists and belongs to the project
	err := k.retryK8sOperation(ctx, "GetComponent", func() error {
		return k.client.Get(ctx, key, component)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Component doesn't exist, consider it already deleted
			return nil
		}
		return fmt.Errorf("failed to get component for deletion: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projName {
		return fmt.Errorf("component does not belong to the specified project")
	}

	// Delete associated builds
	workflowRuns := &v1alpha1.ComponentWorkflowRunList{}
	err = k.retryK8sOperation(ctx, "ListBuilds", func() error {
		return k.client.List(ctx, workflowRuns, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list cwfList for cleanup: %w", err)
	}

	for _, wf := range workflowRuns.Items {
		if wf.Spec.Owner.ProjectName == projName && wf.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteBuild", func() error {
				return k.client.Delete(ctx, &wf)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete build %s: %w", wf.Name, err)
			}
		}
	}

	// Delete associated workloads
	workloadList := &v1alpha1.WorkloadList{}
	err = k.retryK8sOperation(ctx, "ListWorkloads", func() error {
		return k.client.List(ctx, workloadList, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list workloads for cleanup: %w", err)
	}

	for _, workload := range workloadList.Items {
		if workload.Spec.Owner.ProjectName == projName && workload.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteWorkload", func() error {
				return k.client.Delete(ctx, &workload)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete workload %s: %w", workload.Name, err)
			}
		}
	}

	// Delete associated component release
	componentReleases := &v1alpha1.ComponentReleaseList{}
	err = k.retryK8sOperation(ctx, "ListComponentReleases", func() error {
		return k.client.List(ctx, componentReleases, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list component releases for cleanup: %w", err)
	}

	for _, release := range componentReleases.Items {
		if release.Spec.Owner.ProjectName == projName && release.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteComponentRelease", func() error {
				return k.client.Delete(ctx, &release)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete component release %s: %w", release.Name, err)
			}
		}
	}
	// Delete associated component release bindings
	componentReleaseBindings := &v1alpha1.ReleaseBindingList{}
	err = k.retryK8sOperation(ctx, "ListComponentReleaseBindings", func() error {
		return k.client.List(ctx, componentReleaseBindings, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list component release bindings for cleanup: %w", err)
	}

	for _, binding := range componentReleaseBindings.Items {
		if binding.Spec.Owner.ProjectName == projName && binding.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteComponentReleaseBinding", func() error {
				return k.client.Delete(ctx, &binding)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete component release binding %s: %w", binding.Name, err)
			}
		}
	}

	// Finally, delete the component
	err = k.retryK8sOperation(ctx, "DeleteComponent", func() error {
		return k.client.Delete(ctx, component)
	})
	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) TriggerBuild(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error) {
	// Retrieve component and use that to create the build
	component := &v1alpha1.Component{}
	key := client.ObjectKey{
		Name:      agentName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetComponent", func() error {
		return k.client.Get(ctx, key, component)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projName {
		return nil, fmt.Errorf("component does not belong to the specified project")
	}

	// Check if component has workflow configuration
	if component.Spec.Workflow == nil {
		return nil, fmt.Errorf("component %s does not have a workflow configured", component.Name)
	}

	// Extract system parameters from the component's workflow configuration
	var systemParams v1alpha1.SystemParametersValues
	if component.Spec.Workflow.SystemParameters.Repository.URL == "" {
		return nil, fmt.Errorf("component %s workflow does not have repository URL configured", component.Name)
	}

	// Copy system parameters and update the commit
	systemParams = component.Spec.Workflow.SystemParameters
	if commitId != "" {
		// Git commit SHA validation: 7-40 hexadecimal characters
		commitPattern := regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
		if !commitPattern.MatchString(commitId) {
			return nil, fmt.Errorf("invalid commit SHA format: %s", commitId)
		}
	}
	systemParams.Repository.Revision.Commit = commitId

	componentWorkflowRunCR := createComponentWorkflowRunCR(orgName, projName, agentName, systemParams, component)
	err = k.retryK8sOperation(ctx, "TriggerComponentWorkflowRunCR", func() error {
		return k.client.Create(ctx, componentWorkflowRunCR)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to trigger build: %w", err)
	}
	return &models.BuildResponse{
		UUID:        string(componentWorkflowRunCR.UID),
		Name:        componentWorkflowRunCR.Name,
		AgentName:   agentName,
		ProjectName: projName,
		CommitID:    commitId,
		Status:      string(BuildStatusInitiated),
		StartedAt:   time.Now(),
		Branch:      systemParams.Repository.Revision.Branch,
	}, nil
}

func (k *openChoreoSvcClient) DeployAgentComponent(ctx context.Context, orgName string, projName string, componentName string, req *spec.DeployAgentRequest) error {
	exists, err := k.IsAgentComponentExists(ctx, orgName, projName, componentName)
	if err != nil {
		return fmt.Errorf("failed to check agent component existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("agent component %s does not exist in open choreo %s", componentName, projName)
	}
	componentWorkload, err := k.getComponentWorkload(ctx, orgName, projName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component workload: %w", err)
	}
	updateWorkloadSpec(componentWorkload, req)
	err = k.retryK8sOperation(ctx, "UpdateWorkload", func() error {
		return k.client.Update(ctx, componentWorkload)
	})
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}
	// When a Workload is updated and autoDeploy is enabled, it will automatically deploy to the first environment.
	return nil
}

func (k *openChoreoSvcClient) getComponentWorkload(ctx context.Context, orgName string, projectName string, componentName string) (*v1alpha1.Workload, error) {
	workloadList := &v1alpha1.WorkloadList{}
	err := k.retryK8sOperation(ctx, "ListWorkloads", func() error {
		return k.client.List(ctx, workloadList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	var componentWorkload *v1alpha1.Workload
	for i := range workloadList.Items {
		workload := &workloadList.Items[i]
		if workload.Spec.Owner.ComponentName == componentName {
			componentWorkload = workload
			break
		}
	}
	if componentWorkload == nil {
		return nil, fmt.Errorf("workload not found for component %s", componentName)
	}
	return componentWorkload, nil
}

func (k *openChoreoSvcClient) ListComponentWorkflows(ctx context.Context, orgName string, projName string, componentName string) ([]*models.BuildResponse, error) {
	workflowRuns := &v1alpha1.ComponentWorkflowRunList{}
	err := k.retryK8sOperation(ctx, "ListBuilds", func() error {
		return k.client.List(ctx, workflowRuns, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list builds: %w", err)
	}

	buildResponses := make([]*models.BuildResponse, 0, len(workflowRuns.Items))
	for _, workflowRun := range workflowRuns.Items {
		// Only include agent components
		if workflowRun.Spec.Owner.ProjectName != projName || workflowRun.Spec.Owner.ComponentName != componentName {
			continue
		}

		// Set end time if build is completed
		var endedAtTime time.Time
		endTime := findBuildEndTime(workflowRun.Status.Conditions)
		if endTime != nil {
			endedAtTime = endTime.Time
		}

		commit := workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Commit
		if commit == "" {
			commit = "latest"
		}
		buildResponses = append(buildResponses, &models.BuildResponse{
			Name:        workflowRun.Name,
			UUID:        string(workflowRun.UID),
			AgentName:   componentName,
			ProjectName: projName,
			CommitID:    commit,
			Status:      string(determineBuildStatus(workflowRun.Status.Conditions)),
			StartedAt:   workflowRun.CreationTimestamp.Time,
			Image:       workflowRun.Status.ImageStatus.Image,
			Branch:      workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
			EndedAt:     &endedAtTime,
		})
	}

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(buildResponses, func(i, j int) bool {
		return buildResponses[i].StartedAt.After(buildResponses[j].StartedAt)
	})

	return buildResponses, nil
}

func (k *openChoreoSvcClient) GetComponentWorkflow(ctx context.Context, orgName string, projName string, componentName string, buildName string) (*models.BuildDetailsResponse, error) {
	exists, err := k.IsAgentComponentExists(ctx, orgName, projName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to check agent component existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("agent component %s does not exist in open choreo %s", componentName, projName)
	}

	componentWorkflow := &v1alpha1.ComponentWorkflowRun{}
	key := client.ObjectKey{
		Name:      buildName,
		Namespace: orgName,
	}
	err = k.retryK8sOperation(ctx, "GetBuild", func() error {
		return k.client.Get(ctx, key, componentWorkflow)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	// Verify that the build belongs to the specified project and component
	if componentWorkflow.Spec.Owner.ProjectName != projName || componentWorkflow.Spec.Owner.ComponentName != componentName {
		return nil, fmt.Errorf("build does not belong to the specified project or component")
	}

	buildDetails, err := toBuildDetailsResponse(componentWorkflow)
	if err != nil {
		return nil, fmt.Errorf("failed to convert build to build details: %w", err)
	}
	return buildDetails, nil
}

func (k *openChoreoSvcClient) GetAgentDeployments(ctx context.Context, orgName string, pipelineName string, projectName string, componentName string) ([]*models.DeploymentResponse, error) {
	exists, err := k.IsAgentComponentExists(ctx, orgName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to check agent component existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("agent component %s does not exist in open choreo %s", componentName, projectName)
	}

	pipeline, err := k.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline for project %s: %w", pipelineName, err)
	}

	environments, err := k.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	// Create environment order based on the deployment pipeline
	environmentOrder := buildEnvironmentOrder(pipeline.PromotionPaths)

	releaseBindingList := &v1alpha1.ReleaseBindingList{}

	err = k.retryK8sOperation(ctx, "ListReleaseBindings", func() error {
		return k.client.List(ctx, releaseBindingList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	releaseList := &v1alpha1.ReleaseList{}
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
		client.MatchingLabels{
			string(LabelKeyOrganizationName): orgName,
			string(LabelKeyProjectName):      projectName,
			string(LabelKeyComponentName):    componentName,
		},
	}
	err = k.retryK8sOperation(ctx, "ListRelease", func() error {
		return k.client.List(ctx, releaseList, listOpts...)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release: %w", err)
	}

	// Create a map of release bindings by environment for quick lookup
	releaseBindingMap := make(map[string]*v1alpha1.ReleaseBinding)
	for i := range releaseBindingList.Items {
		releaseBinding := &releaseBindingList.Items[i]
		if releaseBinding.Spec.Owner.ProjectName == projectName && releaseBinding.Spec.Owner.ComponentName == componentName {
			releaseEnv := releaseBinding.Spec.Environment
			releaseBindingMap[releaseEnv] = releaseBinding
		}
	}

	// Create a map of releases by environment for quick lookup
	releaseMap := make(map[string]*v1alpha1.Release)
	for i := range releaseList.Items {
		release := &releaseList.Items[i]
		releaseEnv := release.Labels[string(LabelKeyEnvironmentName)]
		releaseMap[releaseEnv] = release
	}

	// Create environment map for quick lookup
	environmentMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range environments {
		environmentMap[env.Name] = env
	}

	// Construct deployment details in the order defined by the pipeline
	var deploymentDetails []*models.DeploymentResponse
	for _, envName := range environmentOrder {
		// Find promotion target environment for this environment
		promotionTargetEnv := findPromotionTargetEnvironment(envName, pipeline.PromotionPaths, environmentMap)
		if releaseBinding, exists := releaseBindingMap[envName]; exists {
			// Ensure corresponding release exists
			if _, releaseExists := releaseMap[envName]; !releaseExists {
				return nil, fmt.Errorf("release not found for environment %s", envName)
			}
			deploymentDetail, err := toDeploymentDetailsResponse(releaseBinding, releaseMap[envName], environmentMap, promotionTargetEnv)
			if err != nil {
				return nil, fmt.Errorf("error creating deployment details for environment %s: %w", envName, err)
			}
			deploymentDetails = append(deploymentDetails, deploymentDetail)
		} else {
			var displayName string
			if env, envExists := environmentMap[envName]; envExists {
				displayName = env.DisplayName
			}

			deploymentDetails = append(deploymentDetails, &models.DeploymentResponse{
				Environment:            envName,
				EnvironmentDisplayName: displayName,
				Status:                 DeploymentStatusNotDeployed,
				Endpoints:              []models.Endpoint{},
			})
		}
	}
	return deploymentDetails, nil
}

func (k *openChoreoSvcClient) GetAgentEndpoints(ctx context.Context, orgName string, projName string, agentName string, environment string) (map[string]models.EndpointsResponse, error) {
	exists, err := k.IsAgentComponentExists(ctx, orgName, projName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to check agent component existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("agent component %s does not exist in open choreo %s", agentName, projName)
	}
	componentWorkload, err := k.getComponentWorkload(ctx, orgName, projName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component workload: %w", err)
	}

	releaseList := &v1alpha1.ReleaseList{}
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
		client.MatchingLabels{
			string(LabelKeyOrganizationName): orgName,
			string(LabelKeyProjectName):      projName,
			string(LabelKeyComponentName):    agentName,
			string(LabelKeyEnvironmentName):  environment,
		},
	}
	err = k.retryK8sOperation(ctx, "ListRelease", func() error {
		return k.client.List(ctx, releaseList, listOpts...)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release: %w", err)
	}
	if len(releaseList.Items) == 0 {
		return nil, fmt.Errorf("no release found")
	}

	// Get the first matching Release (there should only be one per component/environment)
	release := &releaseList.Items[0]
	endpointURLs, err := extractEndpointURLFromEnvRelease(release)
	if err != nil {
		return nil, fmt.Errorf("failed to extract endpoint URLs from release: %w", err)
	}
	if len(endpointURLs) == 0 {
		return nil, fmt.Errorf("no endpoint URLs found in release")
	}

	// Extract endpoint details from workload spec
	endpointDetails := make(map[string]models.EndpointsResponse)

	// Iterate through workload endpoints and match with URLs from release
	for endpointName, endpoint := range componentWorkload.Spec.Endpoints {
		endpointResp := models.EndpointsResponse{}
		endpointResp.Name = endpointName

		// Assuming the first URL corresponds to this endpoint since we have a single endpoint
		endpointResp.URL = endpointURLs[0].URL
		endpointResp.Visibility = endpointURLs[0].Visibility

		// Get schema content from workload endpoint
		if endpoint.Schema != nil {
			endpointResp.Schema = models.EndpointSchema{
				Content: endpoint.Schema.Content,
			}
		}

		endpointDetails[endpointName] = endpointResp
	}

	return endpointDetails, nil
}

func (k *openChoreoSvcClient) ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error) {
	environmentList := &v1alpha1.EnvironmentList{}
	err := k.retryK8sOperation(ctx, "ListEnvironments", func() error {
		return k.client.List(ctx, environmentList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	var environments []*models.EnvironmentResponse
	for _, env := range environmentList.Items {
		environments = append(environments, &models.EnvironmentResponse{
			Name:         env.Name,
			DataplaneRef: env.Spec.DataPlaneRef,
			CreatedAt:    env.CreationTimestamp.Time,
			IsProduction: env.Spec.IsProduction,
			DNSPrefix:    env.Spec.Gateway.DNSPrefix,
			DisplayName:  env.Annotations[string(AnnotationKeyDisplayName)],
		})
	}
	return environments, nil
}

func (k *openChoreoSvcClient) GetEnvironment(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error) {
	environment := &v1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      environmentName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetEnvironment", func() error {
		return k.client.Get(ctx, key, environment)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	envModel := &models.EnvironmentResponse{
		UUID:         string(environment.UID),
		Name:         environment.Name,
		DataplaneRef: environment.Spec.DataPlaneRef,
		CreatedAt:    environment.CreationTimestamp.Time,
		IsProduction: environment.Spec.IsProduction,
		DNSPrefix:    environment.Spec.Gateway.DNSPrefix,
		DisplayName:  environment.Annotations[string(AnnotationKeyDisplayName)],
	}
	return envModel, nil
}

func (k *openChoreoSvcClient) GetDeploymentPipeline(ctx context.Context, orgName string, deploymentPipelineName string) (*models.DeploymentPipelineResponse, error) {
	deploymentPipeline := &v1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      deploymentPipelineName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetDeploymentPipeline", func() error {
		return k.client.Get(ctx, key, deploymentPipeline)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	promotionPaths := buildPromotionPaths(deploymentPipeline.Spec.PromotionPaths)

	dpResponse := &models.DeploymentPipelineResponse{
		Name:           deploymentPipeline.Name,
		DisplayName:    deploymentPipeline.Annotations[string(AnnotationKeyDisplayName)],
		Description:    deploymentPipeline.Annotations[string(AnnotationKeyDescription)],
		OrgName:        orgName,
		CreatedAt:      deploymentPipeline.CreationTimestamp.Time,
		PromotionPaths: promotionPaths,
	}
	return dpResponse, nil
}

func (k *openChoreoSvcClient) GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	// Check if agent component exists
	exists, err := k.IsAgentComponentExists(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to check agent component existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("agent component %s does not exist in open choreo %s", agentName, projectName)
	}

	// Get the workload to extract base environment variables
	componentWorkload, err := k.getComponentWorkload(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component workload: %w", err)
	}

	// Create a map to store environment variables (for easy merging)
	envVarMap := make(map[string]string)

	// Extract base environment variables from workload
	if componentWorkload.Spec.Containers != nil {
		if mainContainer, exists := componentWorkload.Spec.Containers["main"]; exists {
			for _, envVar := range mainContainer.Env {
				envVarMap[envVar.Key] = envVar.Value
			}
		}
	}

	// Get the ReleaseBinding for the specified environment
	releaseBindingList := &v1alpha1.ReleaseBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
		client.MatchingLabels{
			string(LabelKeyOrganizationName): orgName,
			string(LabelKeyProjectName):      projectName,
			string(LabelKeyComponentName):    agentName,
			string(LabelKeyEnvironmentName):  environment,
		},
	}
	err = k.retryK8sOperation(ctx, "ListReleaseBindings", func() error {
		return k.client.List(ctx, releaseBindingList, listOpts...)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	// If a ReleaseBinding exists for this environment, merge its workload overrides
	if len(releaseBindingList.Items) > 0 {
		releaseBinding := &releaseBindingList.Items[0]

		// Override with environment-specific variables from ReleaseBinding
		if releaseBinding.Spec.WorkloadOverrides.Containers != nil {
			if mainContainer, exists := releaseBinding.Spec.WorkloadOverrides.Containers["main"]; exists {
				for _, envVar := range mainContainer.Env {
					// Override or add environment variables
					envVarMap[envVar.Key] = envVar.Value
				}
			}
		}
	}

	// Convert map back to slice
	var envVars []models.EnvVars
	for key, value := range envVarMap {
		envVars = append(envVars, models.EnvVars{
			Key:   key,
			Value: value,
		})
	}

	return envVars, nil
}

func (k *openChoreoSvcClient) GetDeploymentPipelinesForOrganization(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error) {
	deploymentPipelineList := &v1alpha1.DeploymentPipelineList{}
	err := k.retryK8sOperation(ctx, "ListDeploymentPipelines", func() error {
		return k.client.List(ctx, deploymentPipelineList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment pipelines: %w", err)
	}

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(deploymentPipelineList.Items, func(i, j int) bool {
		return deploymentPipelineList.Items[i].CreationTimestamp.After(deploymentPipelineList.Items[j].CreationTimestamp.Time)
	})

	var deploymentPipelines []*models.DeploymentPipelineResponse
	for _, deploymentPipeline := range deploymentPipelineList.Items {
		dpResponse := &models.DeploymentPipelineResponse{
			Name:           deploymentPipeline.Name,
			DisplayName:    deploymentPipeline.Annotations[string(AnnotationKeyDisplayName)],
			Description:    deploymentPipeline.Annotations[string(AnnotationKeyDescription)],
			OrgName:        orgName,
			CreatedAt:      deploymentPipeline.CreationTimestamp.Time,
			PromotionPaths: buildPromotionPaths(deploymentPipeline.Spec.PromotionPaths),
		}
		deploymentPipelines = append(deploymentPipelines, dpResponse)
	}
	return deploymentPipelines, nil
}

func (k *openChoreoSvcClient) CreateProject(ctx context.Context, orgName string, projectName string, deploymentPipelineRef string, projectDisplayName string, projectDescription string) error {
	project := &v1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: orgName,
			Annotations: map[string]string{
				string(AnnotationKeyDisplayName): projectDisplayName,
				string(AnnotationKeyDescription): projectDescription,
			},
		},
		Spec: v1alpha1.ProjectSpec{
			DeploymentPipelineRef: deploymentPipelineRef,
		},
	}
	return k.retryK8sOperation(ctx, "CreateProject", func() error {
		return k.client.Create(ctx, project)
	})
}

func (k *openChoreoSvcClient) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	org := &v1alpha1.Organization{}
	key := client.ObjectKey{
		Name:      orgName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetOrganization", func() error {
		return k.client.Get(ctx, key, org)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	orgModel := &models.OrganizationResponse{
		UUID:        string(org.UID),
		Name:        org.Name,
		Namespace:   org.Name,
		CreatedAt:   org.CreationTimestamp.Time,
		DisplayName: org.Annotations[string(AnnotationKeyDisplayName)],
		Description: org.Annotations[string(AnnotationKeyDescription)],
	}
	return orgModel, nil
}

func (k *openChoreoSvcClient) DeleteProject(ctx context.Context, orgName string, projectName string) error {
	project := &v1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetProject", func() error {
		return k.client.Get(ctx, key, project)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Project doesn't exist, consider it already deleted
			return nil
		}
		return fmt.Errorf("failed to get project: %w", err)
	}

	err = k.retryK8sOperation(ctx, "DeleteProject", func() error {
		return k.client.Delete(ctx, project)
	})
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) ListProjects(ctx context.Context, orgName string) ([]*models.ProjectResponse, error) {
	projectList := &v1alpha1.ProjectList{}
	err := k.retryK8sOperation(ctx, "ListProjects", func() error {
		return k.client.List(ctx, projectList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(projectList.Items, func(i, j int) bool {
		return projectList.Items[i].CreationTimestamp.After(projectList.Items[j].CreationTimestamp.Time)
	})

	var projects []*models.ProjectResponse
	for _, project := range projectList.Items {
		projects = append(projects, &models.ProjectResponse{
			UUID: string(project.UID),
			Name:               project.Name,
			OrgName:            project.Namespace,
			CreatedAt:          project.CreationTimestamp.Time,
			DisplayName:        project.Annotations[string(AnnotationKeyDisplayName)],
			Description:        project.Annotations[string(AnnotationKeyDescription)],
			DeploymentPipeline: project.Spec.DeploymentPipelineRef,
		})
	}
	return projects, nil
}

func (k *openChoreoSvcClient) GetDataplanesForOrganization(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error) {
	dataplaneList := &v1alpha1.DataPlaneList{}
	err := k.retryK8sOperation(ctx, "ListDataplanes", func() error {
		return k.client.List(ctx, dataplaneList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list dataplanes: %w", err)
	}

	var dataplanes []*models.DataPlaneResponse
	for _, dp := range dataplaneList.Items {
		dataplanes = append(dataplanes, &models.DataPlaneResponse{
			Name:        dp.Name,
			OrgName:     orgName,
			CreatedAt:   dp.CreationTimestamp.Time,
			DisplayName: dp.Annotations[string(AnnotationKeyDisplayName)],
			Description: dp.Annotations[string(AnnotationKeyDescription)],
		})
	}
	return dataplanes, nil
}

// findPromotionTargetEnvironment finds the promotion target environment for a given source environment
func findPromotionTargetEnvironment(sourceEnvName string, promotionPaths []models.PromotionPath, environmentMap map[string]*models.EnvironmentResponse) *models.PromotionTargetEnvironment {
	for _, path := range promotionPaths {
		if path.SourceEnvironmentRef != sourceEnvName {
			continue
		}

		// Since promotion is linear, take the first (and only) target
		if len(path.TargetEnvironmentRefs) == 0 {
			return nil
		}

		targetEnvName := path.TargetEnvironmentRefs[0].Name
		var targetDisplayName string
		if env, exists := environmentMap[targetEnvName]; exists {
			targetDisplayName = env.DisplayName
		}
		return &models.PromotionTargetEnvironment{
			Name:        targetEnvName,
			DisplayName: targetDisplayName,
		}
	}
	return nil
}
