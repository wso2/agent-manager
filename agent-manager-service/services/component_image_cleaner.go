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
	"time"
)

// ComponentImageCleaner deletes the container images a deleted component's builds
// pushed to the image registry.
//
// In open-source deployments this is nil and images are left in the registry, as
// before. A deployment whose registry can remove them, for example through a
// platform API that deletes a component's image repository, can inject an
// implementation through app.Options.
//
// Agent Manager calls DeleteComponentImages once the agent's component delete has
// been accepted, in the background, so the call never slows down or fails the
// agent delete. An error is only logged: the component is already gone, so the
// images are orphaned until the deployment's own cleanup finds them.
type ComponentImageCleaner interface {
	// DeleteComponentImages deletes the images of componentName in projectName of
	// the organization ouID. The component is being deleted or already gone when it
	// is called. Implementations must return nil when there is nothing to delete.
	DeleteComponentImages(ctx context.Context, ouID, projectName, componentName string) error
}

// componentImageCleanupTimeout bounds one background DeleteComponentImages call.
const componentImageCleanupTimeout = 2 * time.Minute
