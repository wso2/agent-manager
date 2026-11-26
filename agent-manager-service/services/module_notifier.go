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
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// moduleNotifier is the core's notifications/v1: a module's event, sent to channels the
// module names. The event goes through the outbox and the event dispatcher like any
// other, but to the named destinations instead of to subscriptions.
type moduleNotifier struct {
	destinations repositories.EventRepository
	// outbox is the outbox in a transaction, so a notification is queued only if the
	// caller's transaction commits.
	outbox func(tx *gorm.DB) events.Outbox
	db     *gorm.DB
	now    func() time.Time
}

// NewModuleNotifier returns the core's notifications/v1 implementation: destinations
// looks up the channels a module names, and outbox queues the event in the caller's
// transaction (db when it gives none).
func NewModuleNotifier(destinations repositories.EventRepository, outbox func(tx *gorm.DB) events.Outbox, db *gorm.DB) notifyv1.Notifier {
	return &moduleNotifier{destinations: destinations, outbox: outbox, db: db, now: time.Now}
}

func (n *moduleNotifier) Notify(ctx context.Context, tx *gorm.DB, in notifyv1.Notification) error {
	if len(in.Channels) == 0 {
		return nil
	}
	if tx == nil {
		tx = n.db
	}
	all, err := n.destinations.ListDestinations(ctx, in.OrgID)
	if err != nil {
		return err
	}
	var ids []string
	for i := range all {
		if slices.Contains(in.Channels, all[i].Name) && usableForProject(&all[i], in.Project) {
			ids = append(ids, all[i].ID.String())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	scope, agent := events.ScopeOrg, ""
	switch {
	case in.Project != "" && in.Agent != "":
		scope, agent = events.ScopeAgent, in.Agent
	case in.Project != "":
		scope = events.ScopeProject
	}
	e := events.New(in.ID, in.Type, scope, in.OrgID, in.Project, agent, n.now())
	e.OrgHandle = in.OrgName
	e.Subject = in.Subject
	if in.Data != nil {
		e.Data = in.Data
	}
	e.Destinations = ids
	return n.outbox(tx).Put(ctx, e)
}

func (n *moduleNotifier) Unusable(ctx context.Context, orgID, project string, names []string) ([]notifyv1.Unusable, error) {
	if len(names) == 0 {
		return nil, nil
	}
	all, err := n.destinations.ListDestinations(ctx, orgID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*models.EventDestination, len(all))
	for i := range all {
		byName[all[i].Name] = &all[i]
	}
	var out []notifyv1.Unusable
	for _, name := range names {
		d, ok := byName[name]
		switch {
		case !ok:
			out = append(out, notifyv1.Unusable{Name: name, Missing: true})
		case !usableForProject(d, project):
			out = append(out, notifyv1.Unusable{Name: name})
		}
	}
	return out, nil
}

// usableForProject reports whether a module may send to the destination for project:
// destinations available to all or to the organization always, a projects destination
// only for a project it lists.
func usableForProject(d *models.EventDestination, project string) bool {
	switch d.Availability {
	case models.DestinationAvailableAll, models.DestinationAvailableOrg:
		return true
	case models.DestinationAvailableProjects:
		return project != "" && slices.Contains(d.Projects, project)
	default:
		return false
	}
}
