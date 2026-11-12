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

package ampinit

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

//go:embed templates
var templates embed.FS

const (
	thunderTemplates = "templates/thunder"
	// BootstrapStamp is the file holding a hash of the bootstrap resources. ThunderID's setup runs again
	// whenever it differs from the hash recorded by the last successful setup.
	BootstrapStamp = "bootstrap.sha256"
	// ampResourceServerID is the id of the resource server whose tree defines every amp:* scope.
	ampResourceServerID = "amp-resource-server"
)

// resourceDirs hold the bootstrap resources. users/ is applied only for users ThunderID does not have yet.
var resourceDirs = []string{"bootstrap", "users"}

// resourceServerTrees are the documents that define resource servers, rendered before the rest so the
// others can derive scope lists from them.
var resourceServerTrees = []string{"60-amp-resource-server.yaml", "60-mcp-resource-servers.yaml"}

// WriteThunder writes ThunderID's deployment.yaml, its gate and console config.js, the AMP bootstrap
// resources under bootstrap/, one resource per additional user under users/, and BootstrapStamp into
// dir. Files of an earlier run that this one did not write are removed from bootstrap/ and users/.
func WriteThunder(cfg *ThunderConfig, dir string) error {
	o, err := parseOwner("THUNDER_FILE_OWNER")
	if err != nil {
		return err
	}
	files, err := RenderThunder(cfg)
	if err != nil {
		return err
	}
	for _, sub := range resourceDirs {
		if err := chownDir(filepath.Join(dir, sub), o); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	stamp := sha256.New()
	for _, name := range names {
		// Bootstrap documents carry client secrets and user passwords.
		mode := os.FileMode(0o644)
		if path.Dir(name) != "." {
			mode = 0o600
			_, _ = fmt.Fprintf(stamp, "%s\n%s\n", name, files[name]) // a hash never fails to write
		}
		if err := writeFile(filepath.Join(dir, name), files[name], mode, o); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	for _, sub := range resourceDirs {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if _, ok := files[sub+"/"+e.Name()]; !ok {
				if err := os.Remove(filepath.Join(dir, sub, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	return writeFile(filepath.Join(dir, BootstrapStamp), []byte(hex.EncodeToString(stamp.Sum(nil))+"\n"), 0o644, o)
}

// RenderThunder renders every ThunderID file, keyed by its path relative to the output directory.
func RenderThunder(cfg *ThunderConfig) (map[string][]byte, error) {
	r := &thunderRenderer{cfg: cfg, trees: map[string][]string{}}
	out := map[string][]byte{}
	for _, name := range resourceServerTrees {
		content, err := r.render(path.Join("bootstrap", name))
		if err != nil {
			return nil, err
		}
		if err := r.addTrees(content); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out["bootstrap/"+name] = content
	}
	if len(r.trees[ampResourceServerID]) == 0 {
		return nil, fmt.Errorf("resource server %q defines no scopes", ampResourceServerID)
	}
	err := fs.WalkDir(templates, thunderTemplates, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, thunderTemplates+"/")
		if strings.HasPrefix(rel, "users/") {
			return nil
		}
		rel = strings.Replace(rel, "conf/", "", 1)
		if _, done := out[rel]; done {
			return nil
		}
		content, err := r.render(strings.TrimPrefix(p, thunderTemplates+"/"))
		if err != nil {
			return err
		}
		out[rel] = content
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, u := range cfg.Users {
		content, err := r.renderData("users/user.yaml", u)
		if err != nil {
			return nil, err
		}
		out["users/67-amp-user-"+u.Username+".yaml"] = content
	}
	for name, content := range out {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		dec := yaml.NewDecoder(bytes.NewReader(content))
		for {
			var doc any
			if err := dec.Decode(&doc); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("%s is not valid YAML: %w", name, err)
			}
		}
	}
	return out, nil
}

type thunderRenderer struct {
	cfg *ThunderConfig
	// trees maps a resource server id to the scopes its resource tree defines, in document order.
	trees map[string][]string
}

func (r *thunderRenderer) render(name string) ([]byte, error) {
	return r.renderData(name, r.cfg)
}

func (r *thunderRenderer) renderData(name string, data any) ([]byte, error) {
	src, err := templates.ReadFile(path.Join(thunderTemplates, name))
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"q":                   quote,
		"json":                toJSON,
		"clientScopes":        r.clientScopes,
		"clientScopesWithout": r.clientScopesWithout,
		"clientScopesOnly":    r.clientScopesOnly,
		"rolePermissions":     r.rolePermissions,
	}).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// addTrees records the scopes of every resource server document in content.
func (r *thunderRenderer) addTrees(content []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var doc struct {
			ID        string `yaml:"id"`
			Delimiter string `yaml:"delimiter"`
			Resources []struct {
				Handle  string `yaml:"handle"`
				Parent  string `yaml:"parent"`
				Actions []struct {
					Handle string `yaml:"handle"`
				} `yaml:"actions"`
			} `yaml:"resources"`
		}
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		for _, res := range doc.Resources {
			if res.Parent != rbac.ResourceServer {
				continue
			}
			for _, a := range res.Actions {
				r.trees[doc.ID] = append(r.trees[doc.ID], rbac.Permission(res.Handle+doc.Delimiter+a.Handle).Scope())
			}
		}
	}
}

var oidcScopes = []string{"openid", "profile", "email"}

// clientScopes is every amp scope a client may request, after the OIDC ones, then extra.
func (r *thunderRenderer) clientScopes(extra ...string) []string {
	return slices.Concat(oidcScopes, r.trees[ampResourceServerID], extra)
}

// clientScopesWithout is clientScopes less one resource's scopes.
func (r *thunderRenderer) clientScopesWithout(resource string) []string {
	return slices.DeleteFunc(r.clientScopes(), func(s string) bool { return isResourceScope(s, resource) })
}

// clientScopesOnly is the OIDC scopes and one resource's scopes.
func (r *thunderRenderer) clientScopesOnly(resource string) []string {
	return slices.Concat(oidcScopes, slices.DeleteFunc(slices.Clone(r.trees[ampResourceServerID]),
		func(s string) bool { return !isResourceScope(s, resource) }))
}

func isResourceScope(scope, resource string) bool {
	return strings.HasPrefix(scope, rbac.ResourceServer+":"+resource+":")
}

// rolePermissions is a predefined role's scopes, or with a resource server id, those of them that
// resource server defines.
func (r *thunderRenderer) rolePermissions(role string, resourceServer ...string) ([]string, error) {
	perms, ok := rbac.PredefinedRolePermissions[role]
	if !ok {
		return nil, fmt.Errorf("no predefined role %q", role)
	}
	var within []string
	if len(resourceServer) > 0 {
		var ok bool
		if within, ok = r.trees[resourceServer[0]]; !ok {
			return nil, fmt.Errorf("no resource server %q", resourceServer[0])
		}
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if within == nil || slices.Contains(within, p.Scope()) {
			out = append(out, p.Scope())
		}
	}
	return out, nil
}

// quote is a string as a double-quoted literal, valid in YAML, JavaScript and TOML.
func quote(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
