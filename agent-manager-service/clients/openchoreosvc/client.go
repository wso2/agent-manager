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
	"encoding/base64"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/config"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/spec"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
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
	GetProject(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error)
	IsAgentComponentExists(ctx context.Context, orgName string, projName string, agentName string) (bool, error)
	CreateAgentComponent(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error
	DeleteAgentComponent(ctx context.Context, orgName string, projName string, agentName string) error
	GetAgentComponent(ctx context.Context, orgName string, projName string, agentName string) (*AgentComponent, error)
	ListAgentComponents(ctx context.Context, orgName string, projName string) ([]*AgentComponent, error)
	TriggerBuild(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error)
	DeployBuiltImage(ctx context.Context, orgName string, projName string, componentName string, imageId string) error
	SetupDeployment(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest, envVars []spec.EnvironmentVariable) error
	DeployAgentComponent(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error
	ListAgentBuilds(ctx context.Context, orgName string, projName string, componentName string) ([]*models.BuildResponse, error)
	GetAgentBuild(ctx context.Context, orgName string, projName string, componentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, orgName string, pipelineName string, projName string, componentName string) ([]*models.DeploymentResponse, error)
	GetAgentEndpoints(ctx context.Context, orgName string, projName string, agentName string, environment string) (map[string]models.EndpointsResponse, error)
	GetOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error)
	GetEnvironment(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error)
	GetDeploymentPipeline(ctx context.Context, orgName string, deploymentPipelineName string) (*models.DeploymentPipelineResponse, error)
	GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	CreateOrganization(ctx context.Context, namespaceName string, orgName string, orgDisplayName string) error
	CreateNamespaceForOrganization(ctx context.Context, orgName string) error
	CreateBuildPlaneForOrganization(ctx context.Context, orgName string, buildPlaneName string) error
	CreateDataPlaneForOrganization(ctx context.Context, orgName string, dataPlaneName string) error
	CreateObservabilityEnabledServiceClassForPython(ctx context.Context, orgName string, serviceClassName string) error
	CreateEnvironments(ctx context.Context, orgName string, environmentName string, envDisplayName string, dataplaneName string, isProduction bool, dnsPrefix string) error
	CreateDeploymentPipeline(ctx context.Context, orgName string, pipelineName string, promotionPaths []models.PromotionPath) error
	CreateProject(ctx context.Context, orgName string, projectName string, deploymentPipelineRef string, projectDisplayName string) error
	CleanupOrganizationResources(ctx context.Context, orgName string) error
	ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error)
	GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, orgName string) ([]*models.ProjectResponse, error)
	CreateAPIClassDefaultWithCORS(ctx context.Context, orgName string, apiClassName string) error
	GetDeploymentPipelinesForOrganization(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error)
	DeleteProject(ctx context.Context, orgName string, projectName string) error
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

// getRawKubernetesConfigData returns the cluster name and base64-encoded certificate data from kubeconfig
func getRawKubernetesConfigData() (*KubernetesConfigData, error) {
	if config.GetConfig().IsLocalDevEnv {
		kubeconfigPath := config.GetConfig().KubeConfig

		// Load the kubeconfig using clientcmd to get the decoded data
		kubeconfig, err := clientcmd.LoadFromFile(kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}

		// Get current context info
		currentContext := kubeconfig.CurrentContext
		context, exists := kubeconfig.Contexts[currentContext]
		if !exists {
			return nil, fmt.Errorf("current context not found")
		}

		// Get cluster and user
		cluster, exists := kubeconfig.Clusters[context.Cluster]
		if !exists {
			return nil, fmt.Errorf("cluster not found")
		}

		user, exists := kubeconfig.AuthInfos[context.AuthInfo]
		if !exists {
			return nil, fmt.Errorf("user not found")
		}

		// Convert raw certificate data back to base64 strings
		caCert := base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData)
		clientCert := base64.StdEncoding.EncodeToString(user.ClientCertificateData)
		clientKey := base64.StdEncoding.EncodeToString(user.ClientKeyData)

		return &KubernetesConfigData{
			ClusterName: context.Cluster,
			CACert:      caCert,
			ClientCert:  clientCert,
			ClientKey:   clientKey,
		}, nil
	}

	// Use in-cluster config
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	// Extract certificate data from in-cluster config
	caCert := base64.StdEncoding.EncodeToString(restConfig.CAData)
	clientCert := base64.StdEncoding.EncodeToString(restConfig.CertData)
	clientKey := base64.StdEncoding.EncodeToString(restConfig.KeyData)

	return &KubernetesConfigData{
		ClusterName: "in-cluster",
		CACert:      caCert,
		ClientCert:  clientCert,
		ClientKey:   clientKey,
	}, nil
}

func (k *openChoreoSvcClient) ListAgentComponents(ctx context.Context, orgName string, projName string) ([]*AgentComponent, error) {
	var componentList v1alpha1.ComponentList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	err := k.retryK8sOperation(ctx, "ListComponents", func() error {
		return k.client.List(ctx, &componentList, listOpts...)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list components in namespace %s: %w", orgName, err)
	}

	components := make([]*AgentComponent, 0, len(componentList.Items))
	for _, item := range componentList.Items {
		// Only include agent components
		if item.Labels[string(LabelKeyComponentType)] != AgentComponentType {
			continue
		}
		// Only include components that belong to the specified project
		if item.Spec.Owner.ProjectName == projName {
			components = append(components, toComponentResponse(&item))
		}
	}
	return components, nil
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
		Name:               project.Name,
		OrgName:            project.Namespace,
		DisplayName:        project.Annotations[string(AnnotationKeyDisplayName)],
		Description:        project.Annotations[string(AnnotationKeyDescription)],
		CreatedAt:          project.CreationTimestamp.Time,
		DeploymentPipeline: project.Spec.DeploymentPipelineRef,
	}, nil
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
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projName {
		return nil, fmt.Errorf("component does not belong to the specified project")
	}
	return toComponentResponse(component), nil
}

func (k *openChoreoSvcClient) CreateAgentComponent(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error {
	componentCR := createComponentCR(orgName, projName, req)
	err := k.retryK8sOperation(ctx, "CreateComponent", func() error {
		return k.client.Create(ctx, componentCR)
	})
	if err != nil {
		return fmt.Errorf("failed to create component: %w", err)
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
	buildList := &v1alpha1.BuildList{}
	err = k.retryK8sOperation(ctx, "ListBuilds", func() error {
		return k.client.List(ctx, buildList, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list builds for cleanup: %w", err)
	}

	for _, build := range buildList.Items {
		if build.Spec.Owner.ProjectName == projName && build.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteBuild", func() error {
				return k.client.Delete(ctx, &build)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete build %s: %w", build.Name, err)
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

	// Delete associated services
	serviceList := &v1alpha1.ServiceList{}
	err = k.retryK8sOperation(ctx, "ListServices", func() error {
		return k.client.List(ctx, serviceList, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list services for cleanup: %w", err)
	}

	for _, service := range serviceList.Items {
		if service.Spec.Owner.ProjectName == projName && service.Spec.Owner.ComponentName == agentName {
			err = k.retryK8sOperation(ctx, "DeleteService", func() error {
				return k.client.Delete(ctx, &service)
			})
			if err != nil && client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to delete service %s: %w", service.Name, err)
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

	buildCR := createBuildCR(orgName, projName, agentName, commitId, component)
	err = k.retryK8sOperation(ctx, "CreateBuild", func() error {
		return k.client.Create(ctx, buildCR)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to trigger build: %w", err)
	}
	return &models.BuildResponse{
		UUID:        string(buildCR.UID),
		Name:        buildCR.Name,
		AgentName:   agentName,
		ProjectName: projName,
		CommitID:    commitId,
		Status:      string(ConditionBuildInitiated),
		StartedAt:   time.Now(),
		Branch:      buildCR.Spec.Repository.Revision.Branch,
	}, nil
}

func (k *openChoreoSvcClient) DeployAgentComponent(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
	workloadList, err := k.getWorkloads(ctx, orgName)
	if err != nil {
		return fmt.Errorf("failed to get workloads: %w", err)
	}
	var existingWorkload *v1alpha1.Workload
	for i := range workloadList {
		workload := &workloadList[i]
		if workload.Spec.Owner.ComponentName == componentName {
			existingWorkload = workload
			break
		}
	}
	if existingWorkload == nil {
		return fmt.Errorf("workload not found for component %s", componentName)
	}
	updateWorkloadSpec(existingWorkload, req)
	err = k.retryK8sOperation(ctx, "UpdateWorkload", func() error {
		return k.client.Update(ctx, existingWorkload)
	})
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}

	// If the initial deployment failed we need to update the service resource to link the workload
	err = k.updateServiceResource(ctx, orgName, projName, componentName, existingWorkload.Name)
	if err != nil {
		return fmt.Errorf("failed to update service resource: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) SetupDeployment(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest, envVars []spec.EnvironmentVariable) error {
	endpointDetails, err := createEndpointDetails(req.Name, *req.InputInterface)
	if err != nil {
		return fmt.Errorf("failed to create endpoint details: %w", err)
	}
	// workload is created with a placeholder image, which will be updated during deployment
	_, err = k.createWorkload(ctx, orgName, projName, req.Name, envVars, endpointDetails, "image-id")
	if err != nil {
		return fmt.Errorf("failed to create workload: %w", err)
	}
	// Create service resource for the component without linking the workload
	basePath := "/"
	if req.InputInterface.Type == EndpointTypeCustom && req.InputInterface.CustomOpenAPISpec != nil {
		basePath = req.InputInterface.CustomOpenAPISpec.BasePath
	}
	err = k.createServiceResource(ctx, orgName, projName, req.Name, endpointDetails, basePath, req.RuntimeConfigs.Language)
	if err != nil {
		return fmt.Errorf("failed to create service resource: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) getWorkloads(ctx context.Context, orgName string) ([]v1alpha1.Workload, error) {
	workloadList := &v1alpha1.WorkloadList{}
	err := k.retryK8sOperation(ctx, "ListWorkloads", func() error {
		return k.client.List(ctx, workloadList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	return workloadList.Items, nil
}

// DeployBuiltImage updates the workload with the built image and updates the service binding, this used during build callbacks
func (k *openChoreoSvcClient) DeployBuiltImage(ctx context.Context, orgName string, projName string, componentName string, imageId string) error {
	workloadList, err := k.getWorkloads(ctx, orgName)
	if err != nil {
		return fmt.Errorf("failed to get workloads: %w", err)
	}
	var existingWorkload *v1alpha1.Workload
	for i := range workloadList {
		workload := &workloadList[i]
		if workload.Spec.Owner.ComponentName == componentName {
			existingWorkload = workload
			break
		}
	}

	if existingWorkload == nil {
		return fmt.Errorf("workload not found for component %s", componentName)
	}
	// Update the image in the existing workload
	mainContainer := existingWorkload.Spec.Containers["main"]
	mainContainer.Image = imageId
	existingWorkload.Spec.Containers["main"] = mainContainer

	err = k.retryK8sOperation(ctx, "UpdateWorkload", func() error {
		return k.client.Update(ctx, existingWorkload)
	})
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}

	// Update the workload reference in the service resource
	err = k.updateServiceResource(ctx, orgName, projName, componentName, existingWorkload.Name)
	if err != nil {
		return fmt.Errorf("failed to update service resource: %w", err)
	}
	// once the service and workload resources are linked, oc will create the service binding automatically for the first environment and deploy the workload
	return nil
}

func (k *openChoreoSvcClient) ListAgentBuilds(ctx context.Context, orgName string, projName string, componentName string) ([]*models.BuildResponse, error) {
	builds := &v1alpha1.BuildList{}
	err := k.retryK8sOperation(ctx, "ListBuilds", func() error {
		return k.client.List(ctx, builds, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list builds: %w", err)
	}

	buildResponses := make([]*models.BuildResponse, 0, len(builds.Items))
	for _, build := range builds.Items {
		// Only include agent components
		if build.Spec.Owner.ProjectName != projName || build.Spec.Owner.ComponentName != componentName {
			continue
		}

		// Set end time if build is completed
		var endedAtTime time.Time
		endTime := findBuildEndTime(build.Status.Conditions)
		if endTime != nil {
			endedAtTime = endTime.Time
		}

		commit := build.Spec.Repository.Revision.Commit
		if commit == "" {
			commit = "latest"
		}
		buildResponses = append(buildResponses, &models.BuildResponse{
			Name:        build.Name,
			UUID:        string(build.UID),
			AgentName:   componentName,
			ProjectName: projName,
			CommitID:    commit,
			Status:      GetLatestBuildStatus(build.Status.Conditions),
			StartedAt:   build.CreationTimestamp.Time,
			Image:       build.Status.ImageStatus.Image,
			Branch:      build.Spec.Repository.Revision.Branch,
			EndedAt:     &endedAtTime,
		})
	}

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(buildResponses, func(i, j int) bool {
		return buildResponses[i].StartedAt.After(buildResponses[j].StartedAt)
	})

	return buildResponses, nil
}

func (k *openChoreoSvcClient) GetAgentBuild(ctx context.Context, orgName string, projName string, componentName string, buildName string) (*models.BuildDetailsResponse, error) {
	build := &v1alpha1.Build{}
	key := client.ObjectKey{
		Name:      buildName,
		Namespace: orgName,
	}
	err := k.retryK8sOperation(ctx, "GetBuild", func() error {
		return k.client.Get(ctx, key, build)
	})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	// Verify that the build belongs to the specified project and component
	if build.Spec.Owner.ProjectName != projName || build.Spec.Owner.ComponentName != componentName {
		return nil, fmt.Errorf("build does not belong to the specified project or component")
	}

	buildDetails, err := toBuildDetailsResponse(build)
	if err != nil {
		return nil, fmt.Errorf("failed to convert build to build details: %w", err)
	}
	return buildDetails, nil
}

func (k *openChoreoSvcClient) GetAgentDeployments(ctx context.Context, orgName string, pipelineName string, projectName string, componentName string) ([]*models.DeploymentResponse, error) {
	pipeline, err := k.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline for project %s: %w", pipelineName, err)
	}

	environments, err := k.GetOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	serviceBindings := &v1alpha1.ServiceBindingList{}
	err = k.retryK8sOperation(ctx, "ListServiceBindings", func() error {
		return k.client.List(ctx, serviceBindings, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	// Create a map of service bindings by environment for quick lookup
	serviceBindingMap := make(map[string]*v1alpha1.ServiceBinding)
	for i := range serviceBindings.Items {
		sb := &serviceBindings.Items[i]
		if sb.Spec.Owner.ProjectName == projectName && sb.Spec.Owner.ComponentName == componentName {
			serviceBindingMap[sb.Spec.Environment] = sb
		}
	}

	// Create environment order based on the deployment pipeline
	environmentOrder := buildEnvironmentOrder(pipeline.PromotionPaths)
	log.Printf("Environment order: %v", environmentOrder)

	// Create environment map for quick lookup
	environmentMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range environments {
		environmentMap[env.Name] = env
	}

	// Construct deployment details in the order defined by the pipeline
	var deploymentDetails []*models.DeploymentResponse
	for _, envName := range environmentOrder {
		if sb, exists := serviceBindingMap[envName]; exists {
			deploymentDetails = append(deploymentDetails, toDeploymentDetailsResponse(sb, environmentMap, pipeline.PromotionPaths))
		} else {
			var displayName string
			if env, envExists := environmentMap[envName]; envExists {
				displayName = env.DisplayName
			}

			// Find promotion target environment for this environment
			promotionTargetEnv := findPromotionTargetEnvironment(envName, pipeline.PromotionPaths, environmentMap)
			deploymentDetails = append(deploymentDetails, &models.DeploymentResponse{
				Environment:                envName,
				EnvironmentDisplayName:     displayName,
				PromotionTargetEnvironment: promotionTargetEnv,
				Status:                     DeploymentStatusNotDeployed,
				Endpoints:                  []models.Endpoint{},
			})
		}
	}
	return deploymentDetails, nil
}

func (k *openChoreoSvcClient) GetAgentEndpoints(ctx context.Context, orgName string, projName string, agentName string, environment string) (map[string]models.EndpointsResponse, error) {
	serviceBindings := &v1alpha1.ServiceBindingList{}
	err := k.retryK8sOperation(ctx, "ListServiceBindings", func() error {
		return k.client.List(ctx, serviceBindings, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	endpointDetails := make(map[string]models.EndpointsResponse)
	for _, sb := range serviceBindings.Items {
		if sb.Spec.Owner.ProjectName == projName && sb.Spec.Owner.ComponentName == agentName && sb.Spec.Environment == environment {
			// Check if Status.Endpoints exists and is not nil
			if len(sb.Status.Endpoints) > 0 {
				for _, statusEndpoint := range sb.Status.Endpoints {
					// Get the corresponding spec endpoint for schema info
					var schemaContent string
					if sb.Spec.WorkloadSpec.Endpoints != nil {
						if specEndpoint, exists := sb.Spec.WorkloadSpec.Endpoints[statusEndpoint.Name]; exists {
							schemaContent = specEndpoint.Schema.Content
						}
					}

					// Get URL from Public endpoint
					var endpointURL string
					var epVisibility string
					if statusEndpoint.Public != nil {
						endpointURL = statusEndpoint.Public.URI
						epVisibility = "Public"
					}

					endpointDetails[statusEndpoint.Name] = models.EndpointsResponse{
						Endpoint: models.Endpoint{
							Name:       statusEndpoint.Name,
							URL:        endpointURL,
							Visibility: epVisibility,
						},
						Schema: models.EndpointSchema{
							Content: schemaContent,
						},
					}
				}
			}
		}
	}
	return endpointDetails, nil
}

func (k *openChoreoSvcClient) GetOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error) {
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
			Namespace:    env.Namespace,
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
		Name:         environment.Name,
		Namespace:    environment.Namespace,
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

	promotionPaths := make([]models.PromotionPath, 0, len(deploymentPipeline.Spec.PromotionPaths))
	for _, path := range deploymentPipeline.Spec.PromotionPaths {
		targetRefs := make([]models.TargetEnvironmentRef, 0, len(path.TargetEnvironmentRefs))
		for _, target := range path.TargetEnvironmentRefs {
			targetRefs = append(targetRefs, models.TargetEnvironmentRef{
				Name: target.Name,
			})
		}
		promotionPaths = append(promotionPaths, models.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		})
	}

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
	serviceBindings := &v1alpha1.ServiceBindingList{}
	err := k.retryK8sOperation(ctx, "ListServiceBindings", func() error {
		return k.client.List(ctx, serviceBindings, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	var envVars []models.EnvVars
	for _, sb := range serviceBindings.Items {
		if sb.Spec.Owner.ProjectName != projectName || sb.Spec.Owner.ComponentName != agentName || sb.Spec.Environment != environment {
			continue
		}

		// Access the main container's environment variables
		if sb.Spec.WorkloadSpec.Containers == nil {
			break
		}

		mainContainer, exists := sb.Spec.WorkloadSpec.Containers["main"]
		if !exists {
			break
		}

		for _, envVar := range mainContainer.Env {
			envVars = append(envVars, models.EnvVars{
				Key:   envVar.Key,
				Value: envVar.Value,
			})
		}
		break
	}
	return envVars, nil
}

func (k *openChoreoSvcClient) createWorkload(ctx context.Context, orgName string, projName string, componentName string, envVars []spec.EnvironmentVariable, endpointDetails map[string]spec.EndpointSpec, imageId string) (*string, error) {
	workloadCR := createWorkloadCR(orgName, projName, componentName, envVars, endpointDetails, imageId)
	err := k.retryK8sOperation(ctx, "CreateWorkload", func() error {
		return k.client.Create(ctx, workloadCR)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create workload: %w", err)
	}
	return &workloadCR.Name, nil
}

func (k *openChoreoSvcClient) createServiceResource(ctx context.Context, orgName string, projName string, componentName string, endpointDetails map[string]spec.EndpointSpec, basePath string, language string) error {
	// Check if service already exists
	serviceList := &v1alpha1.ServiceList{}
	err := k.retryK8sOperation(ctx, "ListServices", func() error {
		return k.client.List(ctx, serviceList, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	// Check if service already exists for this component
	for _, service := range serviceList.Items {
		if service.Spec.Owner.ComponentName == componentName && service.Spec.Owner.ProjectName == projName {
			return fmt.Errorf("service already exists for component %s", componentName)
		}
	}

	workloadName := ""
	apis := make(map[string]*v1alpha1.ServiceAPI)
	for epName, epSpec := range endpointDetails {
		api := &v1alpha1.ServiceAPI{
			EndpointTemplateSpec: v1alpha1.EndpointTemplateSpec{
				ClassName: DefaultAPIClassNameWithCORS,
				Type:      v1alpha1.EndpointTypeREST,
				RESTEndpoint: &v1alpha1.RESTEndpoint{
					ExposeLevels: []v1alpha1.RESTOperationExposeLevel{
						v1alpha1.ExposeLevelPublic,
					},
					Backend: v1alpha1.HTTPBackend{
						Port:     epSpec.Port,
						BasePath: basePath,
					},
				},
			},
		}
		apis[epName] = api
	}

	// Create new service
	serviceClassName := DefaultServiceClassName
	if language == string(utils.LanguagePython) {
		serviceClassName = ObservabilityEnabledServiceClassName
	}
	serviceCR := createServiceCR(orgName, projName, componentName, workloadName, serviceClassName, apis)
	err = k.retryK8sOperation(ctx, "CreateService", func() error {
		return k.client.Create(ctx, serviceCR)
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	return nil
}

func (k *openChoreoSvcClient) updateServiceResource(ctx context.Context, orgName string, projName string, componentName string, workloadName string) error {
	// Check if service already exists
	serviceList := &v1alpha1.ServiceList{}
	err := k.retryK8sOperation(ctx, "ListServices", func() error {
		return k.client.List(ctx, serviceList, client.InNamespace(orgName))
	})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	// Check if service already exists for this component
	for _, service := range serviceList.Items {
		if service.Spec.Owner.ComponentName != componentName || service.Spec.Owner.ProjectName != projName {
			continue
		}

		if service.Spec.WorkloadName == workloadName {
			// Service already exists with the correct workload, no need to update
			return nil
		}

		service.Spec.WorkloadName = workloadName
		err = k.retryK8sOperation(ctx, "UpdateService", func() error {
			return k.client.Update(ctx, &service)
		})
		if err != nil {
			return fmt.Errorf("failed to update service: %w", err)
		}
		return nil
	}
	return fmt.Errorf("service not found for component %s", componentName)
}

func (k *openChoreoSvcClient) CreateNamespaceForOrganization(ctx context.Context, orgName string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgName,
		},
	}
	return k.retryK8sOperation(ctx, "CreateNamespaceForOrganization", func() error {
		return k.client.Create(ctx, namespace)
	})
}

func (k *openChoreoSvcClient) CreateOrganization(ctx context.Context, namespaceName string, orgName string, orgDisplayName string) error {
	org := &v1alpha1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgName,
			Namespace: namespaceName,
			Annotations: map[string]string{
				string(AnnotationKeyDisplayName): orgDisplayName,
			},
		},
	}
	return k.retryK8sOperation(ctx, "CreateOrganization", func() error {
		return k.client.Create(ctx, org)
	})
}

func (k *openChoreoSvcClient) GetDeploymentPipelinesForOrganization(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error) {
	deploymentPipelineList := &v1alpha1.DeploymentPipelineList{}
	err := k.retryK8sOperation(ctx, "ListDeploymentPipelines", func() error {
		return k.client.List(ctx, deploymentPipelineList, client.InNamespace(orgName))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment pipelines: %w", err)
	}

	var deploymentPipelines []*models.DeploymentPipelineResponse
	for _, dp := range deploymentPipelineList.Items {
		dpResponse := &models.DeploymentPipelineResponse{
			Name: dp.Name,
		}
		deploymentPipelines = append(deploymentPipelines, dpResponse)
	}
	return deploymentPipelines, nil
}

func (k *openChoreoSvcClient) CreateBuildPlaneForOrganization(ctx context.Context, orgName string, buildPlaneName string) error {
	// Get cluster name and certificate data from kubeconfig (base64-encoded)
	k8sConfig, err := getRawKubernetesConfigData()
	if err != nil {
		return fmt.Errorf("failed to get raw certificate data: %w", err)
	}

	buildPlaneManifest := map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       BuildPlaneKind,
		"metadata": map[string]interface{}{
			"name":      buildPlaneName,
			"namespace": orgName,
			"annotations": map[string]string{
				string(AnnotationKeyDescription): fmt.Sprintf("BuildPlane %s was created for organization %s", buildPlaneName, orgName),
				string(AnnotationKeyDisplayName): fmt.Sprintf("BuildPlane %s", buildPlaneName),
			},
		},
		"spec": map[string]interface{}{
			"kubernetesCluster": map[string]interface{}{
				"name": k8sConfig.ClusterName,
				"credentials": map[string]interface{}{
					"apiServerURL": "https://openchoreo-control-plane:6443",
					"caCert":       k8sConfig.CACert,
					"clientCert":   k8sConfig.ClientCert,
					"clientKey":    k8sConfig.ClientKey,
				},
			},
			"observer": map[string]interface{}{
				"url": "http://observer.openchoreo-observability-plane:8080",
				"authentication": map[string]interface{}{
					"basicAuth": map[string]interface{}{
						"username": config.GetConfig().Observer.Username,
						"password": config.GetConfig().Observer.Password,
					},
				},
			},
		},
	}

	buildPlaneUnstructured := &unstructured.Unstructured{
		Object: buildPlaneManifest,
	}
	buildPlaneUnstructured.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "openchoreo.dev",
		Version: "v1alpha1",
		Kind:    "BuildPlane",
	})

	return k.retryK8sOperation(ctx, "CreateBuildPlane", func() error {
		return k.client.Create(ctx, buildPlaneUnstructured)
	})
}

func (k *openChoreoSvcClient) CreateDataPlaneForOrganization(ctx context.Context, orgName string, dataPlaneName string) error {
	// Get cluster name and certificate data from kubeconfig (base64-encoded)
	k8sConfig, err := getRawKubernetesConfigData()
	if err != nil {
		return fmt.Errorf("failed to get raw certificate data: %w", err)
	}

	dataPlaneManifest := map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       DataPlaneKind,
		"metadata": map[string]interface{}{
			"name":      dataPlaneName,
			"namespace": orgName,
			"annotations": map[string]string{
				string(AnnotationKeyDescription): fmt.Sprintf("DataPlane %s was created for organization %s", dataPlaneName, orgName),
				string(AnnotationKeyDisplayName): fmt.Sprintf("DataPlane %s", dataPlaneName),
			},
		},
		"spec": map[string]interface{}{
			"kubernetesCluster": map[string]interface{}{
				"name": k8sConfig.ClusterName,
				"credentials": map[string]interface{}{
					"apiServerURL": "https://openchoreo-control-plane:6443",
					"caCert":       k8sConfig.CACert,
					"clientCert":   k8sConfig.ClientCert,
					"clientKey":    k8sConfig.ClientKey,
				},
			},
			"registry": map[string]interface{}{
				"prefix":    "registry.openchoreo-data-plane:5000",
				"secretRef": "registry-credentials",
			},
			"gateway": map[string]interface{}{
				"organizationVirtualHost": "openchoreoapis.internal",
				"publicVirtualHost":       "openchoreoapis.localhost",
			},
			"observer": map[string]interface{}{
				"url": "http://observer.openchoreo-observability-plane:8080",
				"authentication": map[string]interface{}{
					"basicAuth": map[string]interface{}{
						"username": config.GetConfig().Observer.Username,
						"password": config.GetConfig().Observer.Password,
					},
				},
			},
		},
	}

	dataPlaneUnstructured := &unstructured.Unstructured{
		Object: dataPlaneManifest,
	}
	dataPlaneUnstructured.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "openchoreo.dev",
		Version: "v1alpha1",
		Kind:    "DataPlane",
	})

	return k.retryK8sOperation(ctx, "CreateDataPlane", func() error {
		return k.client.Create(ctx, dataPlaneUnstructured)
	})
}

// Init container setups OpenTelemetry instrumentation SDK in a shared volume is specific to Python services
func (k *openChoreoSvcClient) CreateObservabilityEnabledServiceClassForPython(ctx context.Context, orgName string, serviceClassName string) error {
	serviceClass := &v1alpha1.ServiceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceClassName,
			Namespace: orgName,
		},
		Spec: v1alpha1.ServiceClassSpec{
			DeploymentTemplate: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: config.GetConfig().OTEL.SDKVolumeName,
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name: MainContainerName,
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      config.GetConfig().OTEL.SDKVolumeName,
										MountPath: config.GetConfig().OTEL.SDKMountPath,
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(DefaultCPURequest),
										corev1.ResourceMemory: resource.MustParse(DefaultMemoryRequest),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(DefaultCPULimit),
										corev1.ResourceMemory: resource.MustParse(DefaultMemoryLimit),
									},
								},
								Env: []corev1.EnvVar{
									{
										Name:  utils.EnvPythonPath,
										Value: config.GetConfig().OTEL.SDKMountPath,
									},
									{
										Name:  utils.EnvAMPTraceloopTraceContent,
										Value: utils.BoolAsString(config.GetConfig().OTEL.TraceContent),
									},
									{
										Name:  utils.EnvAMPOTELExporterOTLPInsecure,
										Value: utils.BoolAsString(config.GetConfig().OTEL.ExporterInsecure),
									},
									{
										Name:  utils.EnvAMPTraceloopMetricsEnabled,
										Value: utils.BoolAsString(config.GetConfig().OTEL.MetricsEnabled),
									},
									{
										Name:  utils.EnvAMPTraceloopTelemetryEnabled,
										Value: utils.BoolAsString(config.GetConfig().OTEL.TelemetryEnabled),
									},
									{
										Name:  utils.EnvAMPOTELExporterOTLPEndpoint,
										Value: config.GetConfig().OTEL.ExporterEndpoint,
									},
								},
							},
						},
						// Add init container to the PodSpec
						InitContainers: []corev1.Container{
							{
								Name:            "setup-instrumentation",
								Image:           config.GetConfig().OTEL.InstrumentationImage,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Env: []corev1.EnvVar{
									{
										Name:  utils.EnvInstrumentationProvider,
										Value: config.GetConfig().OTEL.InstrumentationProvider,
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      config.GetConfig().OTEL.SDKVolumeName,
										MountPath: config.GetConfig().OTEL.SDKMountPath,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return k.retryK8sOperation(ctx, "CreateServiceClass", func() error {
		return k.client.Create(ctx, serviceClass)
	})
}

func (k *openChoreoSvcClient) CreateAPIClassDefaultWithCORS(ctx context.Context, orgName string, apiClassName string) error {
	maxAge := int64(86400)
	apiClass := &v1alpha1.APIClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiClassName,
			Namespace: orgName,
		},
		Spec: v1alpha1.APIClassSpec{
			RESTPolicy: &v1alpha1.RESTAPIPolicy{
				Defaults: &v1alpha1.RESTPolicy{
					CORS: &v1alpha1.CORSPolicy{
						AllowOrigins: []string{config.GetConfig().CORSAllowedOrigin},
						AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
						AllowHeaders: []string{"Content-Type", "Authorization"},
						MaxAge:       &maxAge,
					},
				},
			},
		},
	}
	return k.retryK8sOperation(ctx, "CreateAPIClass", func() error {
		return k.client.Create(ctx, apiClass)
	})
}

func (k *openChoreoSvcClient) CreateEnvironments(ctx context.Context, orgName string, environmentName string, envDisplayName string, dataplaneName string, isProduction bool, dnsPrefix string) error {
	environment := &v1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      environmentName,
			Namespace: orgName,
			Annotations: map[string]string{
				string(AnnotationKeyDisplayName): envDisplayName,
			},
		},
		Spec: v1alpha1.EnvironmentSpec{
			DataPlaneRef: dataplaneName,
			IsProduction: isProduction,
			Gateway: v1alpha1.GatewayConfig{
				DNSPrefix: dnsPrefix,
			},
		},
	}
	return k.retryK8sOperation(ctx, "CreateEnvironment", func() error {
		return k.client.Create(ctx, environment)
	})
}

func (k *openChoreoSvcClient) CreateDeploymentPipeline(ctx context.Context, orgName string, pipelineName string, promotionPaths []models.PromotionPath) error {
	v1alpha1PromotionPaths := make([]v1alpha1.PromotionPath, 0, len(promotionPaths))
	for _, path := range promotionPaths {
		targetRefs := make([]v1alpha1.TargetEnvironmentRef, 0, len(path.TargetEnvironmentRefs))
		for _, target := range path.TargetEnvironmentRefs {
			targetRefs = append(targetRefs, v1alpha1.TargetEnvironmentRef{
				Name:             target.Name,
				RequiresApproval: target.RequiresApproval,
			})
		}
		v1alpha1PromotionPaths = append(v1alpha1PromotionPaths, v1alpha1.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		})
	}

	deploymentPipeline := &v1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pipelineName,
			Namespace: orgName,
			Annotations: map[string]string{
				string(AnnotationKeyDisplayName): pipelineName,
			},
		},
		Spec: v1alpha1.DeploymentPipelineSpec{
			PromotionPaths: v1alpha1PromotionPaths,
		},
	}
	return k.retryK8sOperation(ctx, "CreateDeploymentPipeline", func() error {
		return k.client.Create(ctx, deploymentPipeline)
	})
}

func (k *openChoreoSvcClient) CreateProject(ctx context.Context, orgName string, projectName string, deploymentPipelineRef string, projectDisplayName string) error {
	project := &v1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: orgName,
			Annotations: map[string]string{
				string(AnnotationKeyDisplayName): projectDisplayName,
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

// deleteProjects deletes all projects in the specified namespace
func (k *openChoreoSvcClient) deleteProjects(ctx context.Context, orgName string) []string {
	var errors []string

	projectList := &v1alpha1.ProjectList{}
	if err := k.retryK8sOperation(ctx, "ListProjects", func() error {
		return k.client.List(ctx, projectList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list projects: %v", err))
		return errors
	}

	for _, project := range projectList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteProject", func() error {
			return k.client.Delete(ctx, &project)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete project %s: %v", project.Name, err))
		}
	}

	return errors
}

// deleteDeploymentPipelines deletes all deployment pipelines in the specified namespace
func (k *openChoreoSvcClient) deleteDeploymentPipelines(ctx context.Context, orgName string) []string {
	var errors []string

	pipelineList := &v1alpha1.DeploymentPipelineList{}
	if err := k.retryK8sOperation(ctx, "ListDeploymentPipelines", func() error {
		return k.client.List(ctx, pipelineList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list deployment pipelines: %v", err))
		return errors
	}

	for _, pipeline := range pipelineList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteDeploymentPipeline", func() error {
			return k.client.Delete(ctx, &pipeline)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete deployment pipeline %s: %v", pipeline.Name, err))
		}
	}

	return errors
}

// deleteEnvironments deletes all environments in the specified namespace
func (k *openChoreoSvcClient) deleteEnvironments(ctx context.Context, orgName string) []string {
	var errors []string

	environmentList := &v1alpha1.EnvironmentList{}
	if err := k.retryK8sOperation(ctx, "ListEnvironments", func() error {
		return k.client.List(ctx, environmentList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list environments: %v", err))
		return errors
	}

	for _, environment := range environmentList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteEnvironment", func() error {
			return k.client.Delete(ctx, &environment)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete environment %s: %v", environment.Name, err))
		}
	}

	return errors
}

// deleteServiceClasses deletes all service classes in the specified namespace
func (k *openChoreoSvcClient) deleteServiceClasses(ctx context.Context, orgName string) []string {
	var errors []string

	serviceClassList := &v1alpha1.ServiceClassList{}
	if err := k.retryK8sOperation(ctx, "ListServiceClasses", func() error {
		return k.client.List(ctx, serviceClassList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list service classes: %v", err))
		return errors
	}

	for _, serviceClass := range serviceClassList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteServiceClass", func() error {
			return k.client.Delete(ctx, &serviceClass)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete service class %s: %v", serviceClass.Name, err))
		}
	}

	return errors
}

// deleteDataPlanes deletes all data planes in the specified namespace
func (k *openChoreoSvcClient) deleteDataPlanes(ctx context.Context, orgName string) []string {
	var errors []string

	dataPlaneList := &v1alpha1.DataPlaneList{}
	if err := k.retryK8sOperation(ctx, "ListDataPlanes", func() error {
		return k.client.List(ctx, dataPlaneList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list data planes: %v", err))
		return errors
	}

	for _, dataPlane := range dataPlaneList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteDataPlane", func() error {
			return k.client.Delete(ctx, &dataPlane)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete data plane %s: %v", dataPlane.Name, err))
		}
	}

	return errors
}

// deleteBuildPlanes deletes all build planes in the specified namespace
func (k *openChoreoSvcClient) deleteBuildPlanes(ctx context.Context, orgName string) []string {
	var errors []string

	buildPlaneList := &v1alpha1.BuildPlaneList{}
	if err := k.retryK8sOperation(ctx, "ListBuildPlanes", func() error {
		return k.client.List(ctx, buildPlaneList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list build planes: %v", err))
		return errors
	}

	for _, buildPlane := range buildPlaneList.Items {
		if err := k.retryK8sOperation(ctx, "DeleteBuildPlane", func() error {
			return k.client.Delete(ctx, &buildPlane)
		}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete build plane %s: %v", buildPlane.Name, err))
		}
	}

	return errors
}

// deleteOrganizations deletes the organization with the specified name
func (k *openChoreoSvcClient) deleteOrganizations(ctx context.Context, orgName string) []string {
	var errors []string

	organizationList := &v1alpha1.OrganizationList{}
	if err := k.retryK8sOperation(ctx, "ListOrganizations", func() error {
		return k.client.List(ctx, organizationList, client.InNamespace(orgName))
	}); err != nil {
		errors = append(errors, fmt.Sprintf("failed to list organizations: %v", err))
		return errors
	}

	for _, org := range organizationList.Items {
		if org.Name == orgName {
			if err := k.retryK8sOperation(ctx, "DeleteOrganization", func() error {
				return k.client.Delete(ctx, &org)
			}); err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete organization %s: %v", org.Name, err))
			}
		}
	}

	return errors
}

// deleteNamespace deletes the specified namespace
func (k *openChoreoSvcClient) deleteNamespace(ctx context.Context, orgName string) []string {
	var errors []string

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgName,
		},
	}
	if err := k.retryK8sOperation(ctx, "DeleteNamespace", func() error {
		return k.client.Delete(ctx, namespace)
	}); err != nil {
		// Ignore not found errors for namespace
		if client.IgnoreNotFound(err) != nil {
			errors = append(errors, fmt.Sprintf("failed to delete namespace %s: %v", orgName, err))
		}
	}

	return errors
}

func (k *openChoreoSvcClient) CleanupOrganizationResources(ctx context.Context, orgName string) error {
	// List to track cleanup errors for logging
	var cleanupErrors []string

	// Delete resources in order
	cleanupErrors = append(cleanupErrors, k.deleteProjects(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteDeploymentPipelines(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteEnvironments(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteServiceClasses(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteDataPlanes(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteBuildPlanes(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteOrganizations(ctx, orgName)...)
	cleanupErrors = append(cleanupErrors, k.deleteNamespace(ctx, orgName)...)

	// Log cleanup errors but don't fail the cleanup operation
	if len(cleanupErrors) > 0 {
		log.Printf("Cleanup completed with errors: %v", cleanupErrors)
	}

	return nil
}

func (k *openChoreoSvcClient) ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error) {
	orgList := &v1alpha1.OrganizationList{}
	err := k.retryK8sOperation(ctx, "ListOrganizations", func() error {
		return k.client.List(ctx, orgList)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	var organizations []*models.OrganizationResponse

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(orgList.Items, func(i, j int) bool {
		return orgList.Items[i].CreationTimestamp.After(orgList.Items[j].CreationTimestamp.Time)
	})

	for _, org := range orgList.Items {
		organizations = append(organizations, &models.OrganizationResponse{
			Name:        org.Name,
			Namespace:   org.Name,
			CreatedAt:   org.CreationTimestamp.Time,
			DisplayName: org.Annotations[string(AnnotationKeyDisplayName)],
			Description: org.Annotations[string(AnnotationKeyDescription)],
		})
	}
	return organizations, nil
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
