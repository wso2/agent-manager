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

package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/gitprovider"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	// DefaultLimit is the default number of items per request
	DefaultLimit = 30
	// MaxLimit is the maximum number of items per request
	MaxLimit = 100
)

// RepositoryController defines the interface for repository HTTP handlers
type RepositoryController interface {
	ListBranches(w http.ResponseWriter, r *http.Request)
	ListCommits(w http.ResponseWriter, r *http.Request)
}

type repositoryController struct {
	repositoryService services.RepositoryService
}

// NewRepositoryController creates a new repository controller
func NewRepositoryController(repositoryService services.RepositoryService) RepositoryController {
	return &repositoryController{
		repositoryService: repositoryService,
	}
}

func (c *repositoryController) ListBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Parse request body
	var reqBody spec.ListBranchesRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Error("ListBranches: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if reqBody.Owner == "" {
		log.Error("ListBranches: owner is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field: owner")
		return
	}
	if reqBody.Repository == "" {
		log.Error("ListBranches: repository is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field: repository")
		return
	}

	// Parse pagination from query params
	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		log.Error("ListBranches: invalid pagination params", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Call service
	response, err := c.repositoryService.ListBranches(ctx, reqBody, gitprovider.ProviderGitHub, limit, offset)
	if err != nil {
		log.Error("ListBranches: failed to list branches", "owner", reqBody.Owner, "repository", reqBody.Repository, "error", err)
		handleGitProviderError(w, err)
		return
	}

	log.Info("ListBranches: successfully retrieved branches", "owner", reqBody.Owner, "repository", reqBody.Repository, "count", len(response.Branches))
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// ListCommits handles POST requests to list commits for a repository
func (c *repositoryController) ListCommits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Parse request body
	var reqBody spec.ListCommitsRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Error("ListCommits: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if reqBody.Owner == "" {
		log.Error("ListCommits: owner is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field: owner")
		return
	}
	if reqBody.Repo == "" {
		log.Error("ListCommits: repo is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field: repo")
		return
	}

	// Parse pagination from query params
	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		log.Error("ListCommits: invalid pagination params", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Call service
	response, err := c.repositoryService.ListCommits(ctx, reqBody, gitprovider.ProviderGitHub, limit, offset)
	if err != nil {
		log.Error("ListCommits: failed to list commits", "owner", reqBody.Owner, "repo", reqBody.Repo, "error", err)
		handleGitProviderError(w, err)
		return
	}

	log.Info("ListCommits: successfully retrieved commits", "owner", reqBody.Owner, "repo", reqBody.Repo, "count", len(response.Commits))
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// handleGitProviderError converts git provider errors to HTTP responses
func handleGitProviderError(w http.ResponseWriter, err error) {
	if gitprovider.IsNotFoundError(err) {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Repository not found")
		return
	}
	if gitprovider.IsRateLimitedError(err) {
		utils.WriteErrorResponse(w, http.StatusTooManyRequests, "Rate limit exceeded. Please try again later")
		return
	}
	utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to fetch repository data")
}

// parsePaginationParams parses limit and offset from query parameters
func parsePaginationParams(r *http.Request) (int, int, error) {
	query := r.URL.Query()

	// Parse limit (default: DefaultLimit)
	limit := DefaultLimit
	if limitStr := query.Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid limit: must be a number")
		}
		if parsed <= 0 {
			return 0, 0, fmt.Errorf("invalid limit: must be greater than 0")
		}
		if parsed > MaxLimit {
			return 0, 0, fmt.Errorf("invalid limit: must be at most %d", MaxLimit)
		}
		limit = parsed
	}

	// Parse offset (default: 0)
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsed, err := strconv.Atoi(offsetStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid offset: must be a number")
		}
		if parsed < 0 {
			return 0, 0, fmt.Errorf("invalid offset: must be non-negative")
		}
		offset = parsed
	}

	return limit, offset, nil
}
