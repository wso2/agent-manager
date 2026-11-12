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

// Package fileauth authenticates users against a YAML file of bcrypt password
// hashes and mints JWTs locally, for on-prem deployments that run without an
// identity provider.
//
// It is the only part of the service that issues a user token. Everything
// downstream — issuer and audience allowlists, org identity, scope checks — is
// the same code that validates an IdP token, so a token minted here has to be
// as valid as one from Thunder in every respect except who signed it.
package fileauth

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// UserFile is the on-disk format named by FILE_AUTH_USERS_PATH.
//
// The file is a secret: it holds every operator's password hash. Mount it as a
// Kubernetes Secret or a read-only bind mount, not as part of the image.
type UserFile struct {
	Organization Organization `yaml:"organization"`
	Users        []User       `yaml:"users"`
}

// Organization is the org every user in this file belongs to.
//
// agent-manager-service stores no organizations of its own — it trusts the
// ouId and ouHandle claims on the token, exactly as it trusts Thunder's. So
// these two values are the whole org model in file mode, and nothing
// reconciles them against anything.
type Organization struct {
	// Handle is the org's URL-safe name, stamped as the ouHandle claim.
	Handle string `yaml:"handle"`
	// UUID is the org's stable identifier, stamped as the ouId claim.
	// Both must be non-empty or every request is refused with
	// "missing ou identity in token".
	UUID string `yaml:"uuid"`
}

// User is one login.
type User struct {
	Username string `yaml:"username"`
	// PasswordHash is a bcrypt hash. Generate one with:
	//   htpasswd -bnBC 12 "" 'password' | tr -d ':\n'
	PasswordHash string `yaml:"passwordHash"`
	// Roles are names from rbac.PredefinedRolePermissions. They are expanded
	// to scopes at login; the file never lists scopes directly, so it cannot
	// drift from the catalog.
	Roles []string `yaml:"roles"`
}

// LoadUserFile reads and validates the user file.
//
// Every problem is reported at once rather than one per run: an operator
// editing this file by hand is usually fixing several things, and a loader
// that stops at the first error turns that into several restarts.
func LoadUserFile(path string) (*UserFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read user file %s: %w", path, err)
	}

	var f UserFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse user file %s: %w", path, err)
	}

	var problems []string
	if strings.TrimSpace(f.Organization.Handle) == "" {
		problems = append(problems, "organization.handle is empty")
	}
	if strings.TrimSpace(f.Organization.UUID) == "" {
		problems = append(problems, "organization.uuid is empty")
	}
	if len(f.Users) == 0 {
		problems = append(problems, "users is empty: nobody could log in")
	}

	seen := make(map[string]bool, len(f.Users))
	for i, u := range f.Users {
		where := fmt.Sprintf("users[%d]", i)
		name := strings.TrimSpace(u.Username)
		switch {
		case name == "":
			problems = append(problems, where+": username is empty")
		case seen[name]:
			// Two entries for one name means the second is dead config, and
			// which one wins is an implementation detail nobody should rely on.
			problems = append(problems, fmt.Sprintf("%s: duplicate username %q", where, name))
		default:
			seen[name] = true
		}

		hash := strings.TrimSpace(u.PasswordHash)
		switch {
		case hash == "":
			problems = append(problems, where+": passwordHash is empty")
		case !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$"):
			// A plaintext password here would be compared as a bcrypt hash and
			// always fail, which reads like a wrong password rather than a
			// misconfigured file.
			problems = append(problems, fmt.Sprintf("%s: passwordHash is not a bcrypt hash (expected a $2a$/$2b$/$2y$ prefix)", where))
		}

		if len(u.Roles) == 0 {
			problems = append(problems, where+": roles is empty: this user would hold no permissions")
		}
		for _, role := range u.Roles {
			if _, ok := rbac.PredefinedRolePermissions[role]; !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown role %q (known roles: %s)",
					where, role, strings.Join(KnownRoles(), ", ")))
			}
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid user file %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return &f, nil
}

// KnownRoles lists the role names a user file may name, in a stable order.
func KnownRoles() []string {
	return []string{rbac.RoleAdmin, rbac.RoleDeveloper, rbac.RoleAILead, rbac.RolePlatformEngineer}
}
