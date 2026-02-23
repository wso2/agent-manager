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

package utils

import (
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
)

// Helper functions to convert between spec and models MonitorEvaluator types

// convertSpecEvaluatorsToModels converts []spec.MonitorEvaluator to []models.MonitorEvaluator
func convertSpecEvaluatorsToModels(specEvals []spec.MonitorEvaluator) []models.MonitorEvaluator {
	if len(specEvals) == 0 {
		return []models.MonitorEvaluator{}
	}
	modelsEvals := make([]models.MonitorEvaluator, len(specEvals))
	for i, eval := range specEvals {
		modelsEvals[i] = models.MonitorEvaluator{
			Identifier:  eval.Identifier,
			DisplayName: eval.DisplayName,
			Config:      eval.Config,
		}
	}
	return modelsEvals
}

// convertModelsEvaluatorsToSpec converts []models.MonitorEvaluator to []spec.MonitorEvaluator
func convertModelsEvaluatorsToSpec(modelsEvals []models.MonitorEvaluator) []spec.MonitorEvaluator {
	if len(modelsEvals) == 0 {
		return []spec.MonitorEvaluator{}
	}
	specEvals := make([]spec.MonitorEvaluator, len(modelsEvals))
	for i, eval := range modelsEvals {
		specEvals[i] = spec.MonitorEvaluator{
			Identifier:  eval.Identifier,
			DisplayName: eval.DisplayName,
			Config:      eval.Config,
		}
	}
	return specEvals
}

func ConvertToAgentListResponse(components []*models.AgentResponse) []spec.AgentResponse {
	if len(components) == 0 {
		return []spec.AgentResponse{}
	}
	responses := make([]spec.AgentResponse, len(components))
	for i, component := range components {
		responses[i] = ConvertToAgentResponse(component)
	}
	return responses
}

func ConvertToAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	if component == nil {
		return spec.AgentResponse{}
	}

	if component.Provisioning.Type == string(InternalAgent) {
		return convertToInternalAgentResponse(component)
	}
	return convertToExternalAgentResponse(component)
}

func convertToInternalAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	response := spec.AgentResponse{
		Uuid:        component.UUID,
		Name:        component.Name,
		DisplayName: component.DisplayName,
		Description: component.Description,
		ProjectName: component.ProjectName,
		CreatedAt:   component.CreatedAt,
		Status:      &component.Status,
		Provisioning: spec.Provisioning{
			Type: component.Provisioning.Type,
			Repository: &spec.RepositoryConfig{
				Url:     component.Provisioning.Repository.Url,
				Branch:  component.Provisioning.Repository.Branch,
				AppPath: component.Provisioning.Repository.AppPath,
			},
		},
		AgentType: spec.AgentType{
			Type:    component.Type.Type,
			SubType: &component.Type.SubType,
		},
		InputInterface: convertToInputInterface(component.InputInterface),
		Build:          convertToBuild(component.Build),
	}
	return response
}

func convertToExternalAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	return spec.AgentResponse{
		Uuid:        component.UUID,
		Name:        component.Name,
		DisplayName: component.DisplayName,
		Description: component.Description,
		ProjectName: component.ProjectName,
		CreatedAt:   component.CreatedAt,
		Status:      &component.Status,
		Provisioning: spec.Provisioning{
			Type: component.Provisioning.Type,
		},
		AgentType: spec.AgentType{
			Type: component.Type.Type,
		},
	}
}

func ConvertToBuildResponse(build *models.BuildResponse) spec.BuildResponse {
	if build == nil {
		return spec.BuildResponse{}
	}
	return spec.BuildResponse{
		BuildId:         &build.UUID,
		AgentName:       build.AgentName,
		ProjectName:     build.ProjectName,
		Status:          &build.Status,
		StartedAt:       build.StartedAt,
		ImageId:         &build.ImageId,
		BuildName:       build.Name,
		EndedAt:         build.EndedAt,
		BuildParameters: convertToBuildParameters(build.BuildParameters),
	}
}

func convertToBuildParameters(params models.BuildParameters) spec.BuildParameters {
	return spec.BuildParameters{
		CommitId:        params.CommitID,
		Branch:          params.Branch,
		RepoUrl:         params.RepoUrl,
		AppPath:         params.AppPath,
		Language:        params.Language,
		LanguageVersion: params.LanguageVersion,
		RunCommand:      params.RunCommand,
	}
}

func ConvertToBuildListResponse(builds []*models.BuildResponse) []spec.BuildResponse {
	if len(builds) == 0 {
		return []spec.BuildResponse{}
	}
	responses := make([]spec.BuildResponse, len(builds))
	for i, build := range builds {
		responses[i] = ConvertToBuildResponse(build)
	}
	return responses
}

func ConvertToBuildDetailsResponse(buildDetails *models.BuildDetailsResponse) spec.BuildDetailsResponse {
	if buildDetails == nil {
		return spec.BuildDetailsResponse{}
	}

	steps := make([]spec.BuildStep, len(buildDetails.Steps))
	for i, step := range buildDetails.Steps {
		steps[i] = spec.BuildStep{
			Type:       step.Type,
			Status:     step.Status,
			Message:    step.Message,
			StartedAt:  step.StartedAt,
			FinishedAt: step.FinishedAt,
		}
	}

	response := spec.BuildDetailsResponse{
		BuildId:         &buildDetails.UUID,
		AgentName:       buildDetails.AgentName,
		ProjectName:     buildDetails.ProjectName,
		Status:          &buildDetails.Status,
		StartedAt:       buildDetails.StartedAt,
		ImageId:         &buildDetails.ImageId,
		BuildName:       buildDetails.Name,
		Percent:         &buildDetails.Percent,
		Steps:           steps,
		DurationSeconds: &buildDetails.DurationSeconds,
		EndedAt:         buildDetails.EndedAt,
		BuildParameters: convertToBuildParameters(buildDetails.BuildParameters),
		InputInterface:  convertToInputInterface(buildDetails.InputInterface),
	}

	return response
}

func convertToInputInterface(input *models.InputInterface) *spec.InputInterface {
	if input == nil {
		return nil
	}

	result := &spec.InputInterface{
		Type: input.Type,
		Port: &input.Port,
	}

	if input.Schema != nil {
		result.Schema = &spec.InputInterfaceSchema{
			Path: input.Schema.Path,
		}
	}
	if input.BasePath != "" {
		result.BasePath = &input.BasePath
	}

	return result
}

func convertToBuild(build *models.Build) *spec.Build {
	if build == nil {
		return nil
	}

	if build.Buildpack != nil {
		return &spec.Build{
			BuildpackBuild: &spec.BuildpackBuild{
				Type: build.Type,
				Buildpack: spec.BuildpackConfig{
					Language:        build.Buildpack.Language,
					LanguageVersion: &build.Buildpack.LanguageVersion,
					RunCommand:      &build.Buildpack.RunCommand,
				},
			},
		}
	} else if build.Docker != nil {
		return &spec.Build{
			DockerBuild: &spec.DockerBuild{
				Type: build.Type,
				Docker: spec.DockerConfig{
					DockerfilePath: build.Docker.DockerfilePath,
				},
			},
		}
	}

	return nil
}

func ConvertToDeploymentDetailsResponse(deploymentDetails []*models.DeploymentResponse) map[string]spec.DeploymentDetailsResponse {
	result := make(map[string]spec.DeploymentDetailsResponse)

	if len(deploymentDetails) == 0 {
		return result
	}

	for _, deployment := range deploymentDetails {
		// Convert model endpoints to spec endpoints
		endpoints := make([]spec.DeploymentEndpoint, len(deployment.Endpoints))
		for i, endpoint := range deployment.Endpoints {
			endpoints[i] = spec.DeploymentEndpoint{
				Name:       endpoint.Name,
				Visibility: endpoint.Visibility,
				Url:        endpoint.URL,
			}
		}

		// Create the deployment details response
		var envDisplayName *string
		if deployment.EnvironmentDisplayName != "" {
			envDisplayName = &deployment.EnvironmentDisplayName
		}

		deploymentResponse := spec.DeploymentDetailsResponse{
			ImageId:                deployment.ImageId,
			Status:                 deployment.Status,
			LastDeployed:           deployment.LastDeployedAt,
			Endpoints:              endpoints,
			EnvironmentDisplayName: envDisplayName,
		}

		// Add to result map with environment name as key
		result[deployment.Environment] = deploymentResponse
	}

	return result
}

func ConvertToAgentEndpointResponse(endpointDetails map[string]models.EndpointsResponse) map[string]spec.EndpointConfiguration {
	result := make(map[string]spec.EndpointConfiguration)

	if len(endpointDetails) == 0 {
		return result
	}
	for endpointName, details := range endpointDetails {
		result[endpointName] = spec.EndpointConfiguration{
			Url:          details.URL,
			EndpointName: endpointName,
			Visibility:   details.Visibility,
			Schema: spec.EndpointSchema{
				Content: details.Schema.Content,
			},
		}
	}

	return result
}

func ConvertToEnvironmentListResponse(environments []*models.EnvironmentResponse) []spec.Environment {
	if len(environments) == 0 {
		return []spec.Environment{}
	}

	responses := make([]spec.Environment, len(environments))
	for i, env := range environments {
		responses[i] = ConvertToEnvironmentResponse(env)
	}

	return responses
}

func ConvertToEnvironmentResponse(env *models.EnvironmentResponse) spec.Environment {
	if env == nil {
		return spec.Environment{}
	}

	return spec.Environment{
		Uuid:         env.UUID,
		Name:         env.Name,
		DataplaneRef: env.DataplaneRef,
		IsProduction: env.IsProduction,
		CreatedAt:    env.CreatedAt,
		DisplayName:  &env.DisplayName,
		DnsPrefix:    &env.DNSPrefix,
	}
}

func ConvertToDeploymentPipelinesListResponse(pipelines []*models.DeploymentPipelineResponse, total int32, limit int32, offset int32) spec.DeploymentPipelineListResponse {
	responses := make([]spec.DeploymentPipelineResponse, len(pipelines))
	for i, pipeline := range pipelines {
		responses[i] = ConvertToDeploymentPipelineResponse(pipeline)
	}

	return spec.DeploymentPipelineListResponse{
		DeploymentPipelines: responses,
		Total:               total,
		Limit:               limit,
		Offset:              offset,
	}
}

func ConvertToDeploymentPipelineResponse(pipeline *models.DeploymentPipelineResponse) spec.DeploymentPipelineResponse {
	if pipeline == nil {
		return spec.DeploymentPipelineResponse{}
	}

	promotionPaths := make([]spec.PromotionPath, len(pipeline.PromotionPaths))
	for i, path := range pipeline.PromotionPaths {
		targetRefs := make([]spec.TargetEnvironmentRef, len(path.TargetEnvironmentRefs))
		for j, target := range path.TargetEnvironmentRefs {
			targetRefs[j] = spec.TargetEnvironmentRef{
				Name: target.Name,
			}
		}
		promotionPaths[i] = spec.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		}
	}

	return spec.DeploymentPipelineResponse{
		Name:           pipeline.Name,
		DisplayName:    pipeline.DisplayName,
		Description:    pipeline.Description,
		OrgName:        pipeline.OrgName,
		CreatedAt:      pipeline.CreatedAt,
		PromotionPaths: promotionPaths,
	}
}

func ConvertToOrganizationResponse(org *models.OrganizationResponse) spec.OrganizationResponse {
	if org == nil {
		return spec.OrganizationResponse{}
	}

	return spec.OrganizationResponse{
		Name:        org.Name,
		CreatedAt:   org.CreatedAt,
		DisplayName: org.DisplayName,
		Description: org.Description,
		Namespace:   org.Namespace,
	}
}

func ConvertToOrganizationListItems(org *models.OrganizationResponse) spec.OrganizationListItem {
	if org == nil {
		return spec.OrganizationListItem{}
	}

	return spec.OrganizationListItem{
		Name:      org.Name,
		CreatedAt: org.CreatedAt,
	}
}

func ConvertToOrganizationListResponse(orgs []*models.OrganizationResponse) []spec.OrganizationListItem {
	if len(orgs) == 0 {
		return []spec.OrganizationListItem{}
	}

	responses := make([]spec.OrganizationListItem, len(orgs))
	for i, org := range orgs {
		responses[i] = ConvertToOrganizationListItems(org)
	}

	return responses
}

func ConvertToProjectResponse(project *models.ProjectResponse) spec.ProjectResponse {
	if project == nil {
		return spec.ProjectResponse{}
	}
	return spec.ProjectResponse{
		Uuid:               project.UUID,
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		CreatedAt:          project.CreatedAt,
		DeploymentPipeline: project.DeploymentPipeline,
		OrgName:            project.OrgName,
	}
}

func ConvertToProjectListItem(project *models.ProjectResponse) spec.ProjectListItem {
	if project == nil {
		return spec.ProjectListItem{}
	}

	return spec.ProjectListItem{
		Uuid:        project.UUID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		CreatedAt:   project.CreatedAt,
		OrgName:     project.OrgName,
	}
}

func ConvertToProjectListResponse(projects []*models.ProjectResponse) []spec.ProjectListItem {
	if len(projects) == 0 {
		return []spec.ProjectListItem{}
	}

	responses := make([]spec.ProjectListItem, len(projects))
	for i, project := range projects {
		responses[i] = ConvertToProjectListItem(project)
	}

	return responses
}

func ConvertToLogsResponse(buildLogs models.LogsResponse) spec.LogsResponse {
	logEntries := make([]spec.LogEntry, len(buildLogs.Logs))
	for i, logEntry := range buildLogs.Logs {
		logEntries[i] = spec.LogEntry{
			Timestamp: logEntry.Timestamp,
			Log:       logEntry.Log,
			LogLevel:  logEntry.LogLevel,
		}
	}
	responses := spec.LogsResponse{
		Logs:       logEntries,
		TotalCount: buildLogs.TotalCount,
		TookMs:     buildLogs.TookMs,
	}

	return responses
}

func ConvertToMetricsResponse(metrics *models.MetricsResponse) *spec.MetricsResponse {
	if metrics == nil {
		return nil
	}

	convertDataPoints := func(points []models.TimeValuePoint) []spec.MetricDataPoint {
		result := make([]spec.MetricDataPoint, len(points))
		for i, p := range points {
			result[i] = spec.MetricDataPoint{
				Time:  p.Time,
				Value: p.Value,
			}
		}
		return result
	}

	return &spec.MetricsResponse{
		CpuUsage:       convertDataPoints(metrics.CpuUsage),
		CpuRequests:    convertDataPoints(metrics.CpuRequests),
		CpuLimits:      convertDataPoints(metrics.CpuLimits),
		Memory:         convertDataPoints(metrics.Memory),
		MemoryRequests: convertDataPoints(metrics.MemoryRequests),
		MemoryLimits:   convertDataPoints(metrics.MemoryLimits),
	}
}

func ConvertToDataPlaneListResponse(dataPlanes []*models.DataPlaneResponse) []spec.DataPlane {
	if len(dataPlanes) == 0 {
		return []spec.DataPlane{}
	}

	responses := make([]spec.DataPlane, len(dataPlanes))
	for i, dp := range dataPlanes {
		responses[i] = spec.DataPlane{
			Name:        dp.Name,
			OrgName:     dp.OrgName,
			DisplayName: dp.DisplayName,
			Description: dp.Description,
			CreatedAt:   dp.CreatedAt,
		}
	}

	return responses
}

// ConvertToCreateMonitorRequest converts a spec.CreateMonitorRequest to models.CreateMonitorRequest
func ConvertToCreateMonitorRequest(req *spec.CreateMonitorRequest, projectName, agentName string) *models.CreateMonitorRequest {
	if req == nil {
		return nil
	}

	// Convert IntervalMinutes from *int32 to *int
	var intervalMinutes *int
	if req.IntervalMinutes != nil {
		val := int(*req.IntervalMinutes)
		intervalMinutes = &val
	}

	// Convert SamplingRate from *float32 to *float64
	var samplingRate *float64
	if req.SamplingRate != nil {
		val := float64(*req.SamplingRate)
		samplingRate = &val
	}

	return &models.CreateMonitorRequest{
		Name:            req.Name,
		DisplayName:     req.DisplayName,
		ProjectName:     projectName,
		AgentName:       agentName,
		EnvironmentName: req.EnvironmentName,
		Evaluators:      convertSpecEvaluatorsToModels(req.Evaluators),
		Type:            req.Type,
		IntervalMinutes: intervalMinutes,
		TraceStart:      req.TraceStart,
		TraceEnd:        req.TraceEnd,
		SamplingRate:    samplingRate,
	}
}

// ConvertToUpdateMonitorRequest converts a spec.UpdateMonitorRequest to models.UpdateMonitorRequest
func ConvertToUpdateMonitorRequest(req *spec.UpdateMonitorRequest) *models.UpdateMonitorRequest {
	if req == nil {
		return nil
	}

	// Convert IntervalMinutes from *int32 to *int
	var intervalMinutes *int
	if req.IntervalMinutes != nil {
		val := int(*req.IntervalMinutes)
		intervalMinutes = &val
	}

	// Convert SamplingRate from *float32 to *float64
	var samplingRate *float64
	if req.SamplingRate != nil {
		val := float64(*req.SamplingRate)
		samplingRate = &val
	}

	// Convert Evaluators - handle empty vs nil
	var evaluators *[]models.MonitorEvaluator
	if len(req.Evaluators) > 0 {
		converted := convertSpecEvaluatorsToModels(req.Evaluators)
		evaluators = &converted
	}

	return &models.UpdateMonitorRequest{
		DisplayName:     req.DisplayName,
		Evaluators:      evaluators,
		IntervalMinutes: intervalMinutes,
		SamplingRate:    samplingRate,
	}
}

// ConvertToMonitorResponse converts a models.MonitorResponse to spec.MonitorResponse
func ConvertToMonitorResponse(monitor *models.MonitorResponse) spec.MonitorResponse {
	if monitor == nil {
		return spec.MonitorResponse{}
	}

	// Convert IntervalMinutes from *int to *int32
	var intervalMinutes *int32
	if monitor.IntervalMinutes != nil {
		val := int32(*monitor.IntervalMinutes)
		intervalMinutes = &val
	}

	response := spec.MonitorResponse{
		Id:              monitor.ID,
		Name:            monitor.Name,
		DisplayName:     monitor.DisplayName,
		Type:            monitor.Type,
		OrgName:         monitor.OrgName,
		ProjectName:     monitor.ProjectName,
		AgentName:       monitor.AgentName,
		EnvironmentName: monitor.EnvironmentName,
		Evaluators:      convertModelsEvaluatorsToSpec(monitor.Evaluators),
		IntervalMinutes: intervalMinutes,
		NextRunTime:     monitor.NextRunTime,
		TraceStart:      monitor.TraceStart,
		TraceEnd:        monitor.TraceEnd,
		SamplingRate:    float32(monitor.SamplingRate),
		Status:          string(monitor.Status),
		CreatedAt:       monitor.CreatedAt,
	}

	// Convert LatestRun if present
	if monitor.LatestRun != nil {
		latestRun := ConvertToMonitorRunResponse(monitor.LatestRun)
		response.LatestRun = &latestRun
	}

	return response
}

// ConvertToMonitorListResponse converts a models.MonitorListResponse to spec.MonitorListResponse
func ConvertToMonitorListResponse(monitorList *models.MonitorListResponse) spec.MonitorListResponse {
	if monitorList == nil || len(monitorList.Monitors) == 0 {
		return spec.MonitorListResponse{
			Monitors: []spec.MonitorResponse{},
			Total:    0,
		}
	}

	responses := make([]spec.MonitorResponse, len(monitorList.Monitors))
	for i, monitor := range monitorList.Monitors {
		responses[i] = ConvertToMonitorResponse(&monitor)
	}

	return spec.MonitorListResponse{
		Monitors: responses,
		Total:    int32(monitorList.Total),
	}
}

// ConvertToMonitorRunResponse converts a models.MonitorRunResponse to spec.MonitorRunResponse
func ConvertToMonitorRunResponse(run *models.MonitorRunResponse) spec.MonitorRunResponse {
	if run == nil {
		return spec.MonitorRunResponse{}
	}

	response := spec.MonitorRunResponse{
		Id:           run.ID,
		Evaluators:   convertModelsEvaluatorsToSpec(run.Evaluators),
		TraceStart:   run.TraceStart,
		TraceEnd:     run.TraceEnd,
		StartedAt:    run.StartedAt,
		CompletedAt:  run.CompletedAt,
		Status:       run.Status,
		ErrorMessage: run.ErrorMessage,
	}

	// Add MonitorName if present
	if run.MonitorName != "" {
		response.MonitorName = &run.MonitorName
	}

	return response
}

// ConvertToMonitorRunListResponse converts a models.MonitorRunsListResponse to spec.MonitorRunListResponse
func ConvertToMonitorRunListResponse(runList *models.MonitorRunsListResponse) spec.MonitorRunListResponse {
	if runList == nil || len(runList.Runs) == 0 {
		return spec.MonitorRunListResponse{
			Runs:  []spec.MonitorRunResponse{},
			Total: 0,
		}
	}

	responses := make([]spec.MonitorRunResponse, len(runList.Runs))
	for i, run := range runList.Runs {
		responses[i] = ConvertToMonitorRunResponse(&run)
	}

	return spec.MonitorRunListResponse{
		Runs:  responses,
		Total: int32(runList.Total),
	}
}

// ConvertToMonitorScoresResponse converts a models.MonitorScoresResponse to spec.MonitorScoresResponse
func ConvertToMonitorScoresResponse(response *models.MonitorScoresResponse) spec.MonitorScoresResponse {
	if response == nil {
		return spec.MonitorScoresResponse{
			MonitorName: "",
			TimeRange:   spec.TimeRange{},
			Evaluators:  []spec.EvaluatorScoreSummary{},
		}
	}

	evaluators := make([]spec.EvaluatorScoreSummary, len(response.Evaluators))
	for i, eval := range response.Evaluators {
		evaluators[i] = spec.EvaluatorScoreSummary{
			EvaluatorName: eval.EvaluatorName,
			Level:         eval.Level,
			Count:         int32(eval.Count),
			ErrorCount:    int32(eval.ErrorCount),
			Aggregations:  eval.Aggregations,
		}
	}

	return spec.MonitorScoresResponse{
		MonitorName: response.MonitorName,
		TimeRange: spec.TimeRange{
			Start: response.TimeRange.Start,
			End:   response.TimeRange.End,
		},
		Evaluators: evaluators,
	}
}

// ConvertToMonitorRunScoresResponse converts a models.MonitorRunScoresResponse to spec.MonitorRunScoresResponse
func ConvertToMonitorRunScoresResponse(response *models.MonitorRunScoresResponse) spec.MonitorRunScoresResponse {
	if response == nil {
		return spec.MonitorRunScoresResponse{
			RunId:       "",
			MonitorName: "",
			Evaluators:  []spec.EvaluatorScoreSummary{},
		}
	}

	evaluators := make([]spec.EvaluatorScoreSummary, len(response.Evaluators))
	for i, eval := range response.Evaluators {
		evaluators[i] = spec.EvaluatorScoreSummary{
			EvaluatorName: eval.EvaluatorName,
			Level:         eval.Level,
			Count:         int32(eval.Count),
			ErrorCount:    int32(eval.ErrorCount),
			Aggregations:  eval.Aggregations,
		}
	}

	return spec.MonitorRunScoresResponse{
		RunId:       response.RunID,
		MonitorName: response.MonitorName,
		Evaluators:  evaluators,
	}
}

// ConvertToTimeSeriesResponse converts a models.TimeSeriesResponse to spec.TimeSeriesResponse
func ConvertToTimeSeriesResponse(response *models.TimeSeriesResponse) spec.TimeSeriesResponse {
	if response == nil {
		return spec.TimeSeriesResponse{
			MonitorName:   "",
			EvaluatorName: "",
			Granularity:   "",
			Points:        []spec.TimeSeriesPoint{},
		}
	}

	points := make([]spec.TimeSeriesPoint, len(response.Points))
	for i, point := range response.Points {
		points[i] = spec.TimeSeriesPoint{
			Timestamp:    point.Timestamp,
			Count:        int32(point.Count),
			ErrorCount:   int32(point.ErrorCount),
			Aggregations: point.Aggregations,
		}
	}

	return spec.TimeSeriesResponse{
		MonitorName:   response.MonitorName,
		EvaluatorName: response.EvaluatorName,
		Granularity:   response.Granularity,
		Points:        points,
	}
}

// ConvertToTraceScoresResponse converts a models.TraceScoresResponse to spec.TraceScoresResponse
func ConvertToTraceScoresResponse(response *models.TraceScoresResponse) spec.TraceScoresResponse {
	if response == nil {
		return spec.TraceScoresResponse{
			TraceId:  "",
			Monitors: []spec.MonitorTraceGroup{},
		}
	}

	monitors := make([]spec.MonitorTraceGroup, len(response.Monitors))
	for i, monitor := range response.Monitors {
		evaluators := make([]spec.EvaluatorTraceGroup, len(monitor.Evaluators))
		for j, eval := range monitor.Evaluators {
			scores := make([]spec.ScoreItem, len(eval.Scores))
			for k, score := range eval.Scores {
				// Convert score from *float64 to *float32 for spec
				var scoreFloat32 *float32
				if score.Score != nil {
					f32 := float32(*score.Score)
					scoreFloat32 = &f32
				}

				scores[k] = spec.ScoreItem{
					SpanId:      *spec.NewNullableString(score.SpanID),
					Score:       *spec.NewNullableFloat32(scoreFloat32),
					Explanation: *spec.NewNullableString(score.Explanation),
					Metadata:    score.Metadata,
					Error:       *spec.NewNullableString(score.Error),
				}
			}

			evaluators[j] = spec.EvaluatorTraceGroup{
				EvaluatorName: eval.EvaluatorName,
				Level:         eval.Level,
				Scores:        scores,
			}
		}

		monitors[i] = spec.MonitorTraceGroup{
			MonitorName: monitor.MonitorName,
			MonitorId:   monitor.MonitorID,
			RunId:       monitor.RunID,
			Evaluators:  evaluators,
		}
	}

	return spec.TraceScoresResponse{
		TraceId:  response.TraceID,
		Monitors: monitors,
	}
}
