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

package growthanalytics

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// taxonomy.yaml is the contract for every event this service sends to Moesif.
// The tests below keep it honest in two directions: the file must conform to
// the WSO2 cloud common feature-taxonomy skeleton
// (https://wso2.com/schemas/feature-taxonomy/v1), and every event the code
// emits — Track wrappers in api/ and the console allowlist in console.go —
// must be declared in it with the exact dimension keys and static values sent.
// Controller SetDimension calls contribute keys and literal values too;
// computed runtime values cannot be resolved by this source-level check.
//
// The emitted codes are already in production data, so when these disagree
// the fix is to the YAML, not to the code.

const (
	taxonomyPath   = "taxonomy.yaml"
	apiRoutesDir   = "../../api"
	controllersDir = "../../controllers"
)

// The structs mirror the skeleton field for field. Decoding with KnownFields
// rejects any key the skeleton does not define.
type taxonomyFile struct {
	Schema struct {
		Version string `yaml:"version"`
		ID      string `yaml:"id"`
	} `yaml:"schema"`
	Product struct {
		ID          string `yaml:"id"`
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
		TeamOwner   string `yaml:"team_owner"`
		Category    string `yaml:"category"`
		IsActive    *bool  `yaml:"is_active"`
	} `yaml:"product"`
	Categories []taxonomyCategory `yaml:"categories"`
	Features   []taxonomyFeature  `yaml:"features"`
}

type taxonomyCategory struct {
	ID               string             `yaml:"id"`
	Name             string             `yaml:"name"`
	Description      string             `yaml:"description"`
	ParentCategoryID *string            `yaml:"parent_category_id"`
	SubCategories    []taxonomyCategory `yaml:"sub_categories"`
}

type taxonomyFeature struct {
	Code         string `yaml:"code"`
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Category     string `yaml:"category"`
	Tier         string `yaml:"tier"`
	Status       string `yaml:"status"`
	Availability struct {
		IntroducedIn        string  `yaml:"introduced_in"`
		DeprecatedIn        *string `yaml:"deprecated_in"`
		VersionTierOverride *string `yaml:"version_tier_override"`
	} `yaml:"availability"`
	Flags []struct {
		Key               string  `yaml:"key"`
		IsEnabled         bool    `yaml:"is_enabled"`
		RolloutPercentage int     `yaml:"rollout_percentage"`
		RolloutRegions    string  `yaml:"rollout_regions"`
		EnabledFrom       *string `yaml:"enabled_from"`
		EnabledUntil      *string `yaml:"enabled_until"`
	} `yaml:"flags"`
	Tracking struct {
		EventName     string              `yaml:"event_name"`
		EventCategory string              `yaml:"event_category"`
		Platforms     []string            `yaml:"platforms"`
		Dimensions    []taxonomyDimension `yaml:"dimensions"`
		Aggregations  []struct {
			Type      string  `yaml:"type"`
			Label     string  `yaml:"label"`
			Dimension *string `yaml:"dimension"`
		} `yaml:"aggregations"`
	} `yaml:"tracking"`
	SubFeatures []taxonomyFeature `yaml:"sub_features"`
}

type taxonomyDimension struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Values      []string `yaml:"values"`
	Required    bool     `yaml:"required"`
	Description string   `yaml:"description"`
}

var (
	semverPattern        = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	versionPattern       = regexp.MustCompile(`^\d+\.\d+\.\d+[\w.-]*$`)
	slugPattern          = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	featureCodePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*$`)
	dimensionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	flagKeyPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	timestampPattern     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
)

var (
	productCategories = []string{"api-management", "integration", "identity-access", "analytics", "developer-platform", "cloud", "security"}
	tiers             = []string{"core", "premium", "enterprise"}
	statuses          = []string{"stable", "beta", "experimental", "deprecated"}
	eventCategories   = []string{"discovery", "engagement", "adoption", "abandonment", "lifecycle"}
	platforms         = []string{"web", "mobile-ios", "mobile-android", "desktop", "api"}
	dimensionTypes    = []string{"string", "integer", "float", "boolean", "enum"}
	aggregationTypes  = []string{"count", "count_distinct", "sum", "avg", "p50", "p95", "p99"}
	rolloutRegions    = []string{"all", "NA", "LATAM", "AFRICA", "ME", "EU", "APAC"}
)

func loadTaxonomy(t *testing.T) taxonomyFile {
	t.Helper()
	raw, err := os.ReadFile(taxonomyPath)
	if err != nil {
		t.Fatalf("read %s: %v", taxonomyPath, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var tx taxonomyFile
	if err := dec.Decode(&tx); err != nil {
		t.Fatalf("%s does not match the feature-taxonomy skeleton: %v", taxonomyPath, err)
	}
	return tx
}

// allFeatures flattens features and their sub-features, keyed by code.
func allFeatures(t *testing.T, tx taxonomyFile) map[string]taxonomyFeature {
	t.Helper()
	out := map[string]taxonomyFeature{}
	var walk func(fs []taxonomyFeature)
	walk = func(fs []taxonomyFeature) {
		for _, f := range fs {
			if _, dup := out[f.Code]; dup {
				t.Errorf("feature code %q is declared more than once", f.Code)
			}
			out[f.Code] = f
			walk(f.SubFeatures)
		}
	}
	walk(tx.Features)
	return out
}

func (f taxonomyFeature) dimension(name string) (taxonomyDimension, bool) {
	for _, d := range f.Tracking.Dimensions {
		if d.Name == name {
			return d, true
		}
	}
	return taxonomyDimension{}, false
}

// TestTaxonomyConformsToSkeleton checks the rules the skeleton states beyond
// field names: required fields, slugs, enums, version patterns, and that a
// feature's category is the second segment of its code.
//
// One skeleton rule is deliberately not checked: the event_name pattern
// ^[a-z][a-z0-9.]*$ forbids hyphens, yet the skeleton's own example
// ("apim.api-designer.openapi-import") and every live AMP event use them.
// event_name is instead required to equal the code, which is checked against
// the code pattern.
func TestTaxonomyConformsToSkeleton(t *testing.T) {
	tx := loadTaxonomy(t)

	if !semverPattern.MatchString(tx.Schema.Version) {
		t.Errorf("schema.version %q is not major.minor.patch", tx.Schema.Version)
	}
	if tx.Schema.ID == "" {
		t.Error("schema.id is required")
	}

	p := tx.Product
	if !slugPattern.MatchString(p.ID) {
		t.Errorf("product.id %q is not a slug", p.ID)
	}
	if p.Name == "" {
		t.Error("product.name is required")
	}
	if !semverPattern.MatchString(p.Version) {
		t.Errorf("product.version %q is not major.minor.patch", p.Version)
	}
	if p.Category != "" && !slices.Contains(productCategories, p.Category) {
		t.Errorf("product.category %q is not one of %v", p.Category, productCategories)
	}

	categories := map[string]bool{}
	var collect func(cs []taxonomyCategory)
	collect = func(cs []taxonomyCategory) {
		for _, c := range cs {
			if !slugPattern.MatchString(c.ID) {
				t.Errorf("category id %q is not a slug", c.ID)
			}
			if c.Name == "" {
				t.Errorf("category %q has no name", c.ID)
			}
			if categories[c.ID] {
				t.Errorf("category id %q is declared more than once", c.ID)
			}
			categories[c.ID] = true
			collect(c.SubCategories)
		}
	}
	collect(tx.Categories)
	for _, c := range tx.Categories {
		if c.ParentCategoryID != nil && !categories[*c.ParentCategoryID] {
			t.Errorf("category %q has unknown parent_category_id %q", c.ID, *c.ParentCategoryID)
		}
	}

	flagKeys := map[string]bool{}
	var check func(f taxonomyFeature, parent string)
	check = func(f taxonomyFeature, parent string) {
		code := f.Code
		if !featureCodePattern.MatchString(code) {
			t.Errorf("%s: code does not match %s", code, featureCodePattern)
		}
		segments := strings.Split(code, ".")
		if segments[0] != p.ID {
			t.Errorf("%s: code must start with the product id %q", code, p.ID)
		}
		if !categories[f.Category] {
			t.Errorf("%s: category %q is not declared", code, f.Category)
		}
		if len(segments) < 3 || segments[1] != f.Category {
			t.Errorf("%s: second code segment must equal category %q", code, f.Category)
		}
		if parent != "" && !strings.HasPrefix(code, parent+".") {
			t.Errorf("%s: sub-feature code must be prefixed with its parent %q", code, parent)
		}
		if f.Name == "" {
			t.Errorf("%s: name is required", code)
		}
		if !slices.Contains(tiers, f.Tier) {
			t.Errorf("%s: tier %q is not one of %v", code, f.Tier, tiers)
		}
		if !slices.Contains(statuses, f.Status) {
			t.Errorf("%s: status %q is not one of %v", code, f.Status, statuses)
		}

		av := f.Availability
		if !versionPattern.MatchString(av.IntroducedIn) {
			t.Errorf("%s: availability.introduced_in %q is not a version", code, av.IntroducedIn)
		}
		if av.DeprecatedIn != nil && !versionPattern.MatchString(*av.DeprecatedIn) {
			t.Errorf("%s: availability.deprecated_in %q is not a version", code, *av.DeprecatedIn)
		}
		if av.VersionTierOverride != nil && !slices.Contains(tiers, *av.VersionTierOverride) {
			t.Errorf("%s: availability.version_tier_override %q is not one of %v", code, *av.VersionTierOverride, tiers)
		}

		for _, fl := range f.Flags {
			if !flagKeyPattern.MatchString(fl.Key) {
				t.Errorf("%s: flag key %q does not match %s", code, fl.Key, flagKeyPattern)
			}
			if flagKeys[fl.Key] {
				t.Errorf("%s: flag key %q is used by more than one feature", code, fl.Key)
			}
			flagKeys[fl.Key] = true
			if fl.RolloutPercentage < 0 || fl.RolloutPercentage > 100 {
				t.Errorf("%s: flag %q rollout_percentage %d is outside 0-100", code, fl.Key, fl.RolloutPercentage)
			}
			if fl.RolloutRegions != "" && !slices.Contains(rolloutRegions, fl.RolloutRegions) {
				t.Errorf("%s: flag %q rollout_regions %q is not one of %v", code, fl.Key, fl.RolloutRegions, rolloutRegions)
			}
			for _, ts := range []*string{fl.EnabledFrom, fl.EnabledUntil} {
				if ts != nil && !timestampPattern.MatchString(*ts) {
					t.Errorf("%s: flag %q timestamp %q is not ISO 8601 UTC", code, fl.Key, *ts)
				}
			}
		}

		tr := f.Tracking
		if tr.EventName != code {
			t.Errorf("%s: tracking.event_name %q must equal the code", code, tr.EventName)
		}
		if !slices.Contains(eventCategories, tr.EventCategory) {
			t.Errorf("%s: event_category %q is not one of %v", code, tr.EventCategory, eventCategories)
		}
		for _, pl := range tr.Platforms {
			if !slices.Contains(platforms, pl) {
				t.Errorf("%s: platform %q is not one of %v", code, pl, platforms)
			}
		}
		dims := map[string]bool{}
		for _, d := range tr.Dimensions {
			if !dimensionNamePattern.MatchString(d.Name) {
				t.Errorf("%s: dimension name %q does not match %s", code, d.Name, dimensionNamePattern)
			}
			if dims[d.Name] {
				t.Errorf("%s: dimension %q is declared more than once", code, d.Name)
			}
			dims[d.Name] = true
			if !slices.Contains(dimensionTypes, d.Type) {
				t.Errorf("%s: dimension %q type %q is not one of %v", code, d.Name, d.Type, dimensionTypes)
			}
			if d.Type == "enum" && len(d.Values) == 0 {
				t.Errorf("%s: enum dimension %q needs values", code, d.Name)
			}
		}
		for _, a := range tr.Aggregations {
			if !slices.Contains(aggregationTypes, a.Type) {
				t.Errorf("%s: aggregation type %q is not one of %v", code, a.Type, aggregationTypes)
			}
			if a.Label == "" {
				t.Errorf("%s: aggregation %q needs a label", code, a.Type)
			}
			if a.Type != "count" && (a.Dimension == nil || !dims[*a.Dimension]) {
				t.Errorf("%s: %s aggregation must name a declared dimension", code, a.Type)
			}
		}

		for _, sf := range f.SubFeatures {
			check(sf, code)
		}
	}
	for _, f := range tx.Features {
		check(f, "")
	}
}

// TestTaxonomyDeclaresConsoleActions pins console.go's allowlist to the
// taxonomy: each forwarded action is a declared feature with exactly the
// dimension keys the service lets through.
func TestTaxonomyDeclaresConsoleActions(t *testing.T) {
	features := allFeatures(t, loadTaxonomy(t))
	for action, allowed := range consoleActions {
		f, ok := features[action]
		if !ok {
			t.Errorf("console action %q is not declared in %s", action, taxonomyPath)
			continue
		}
		var declared, sent []string
		for _, d := range f.Tracking.Dimensions {
			declared = append(declared, d.Name)
		}
		for k := range allowed {
			sent = append(sent, k)
		}
		sort.Strings(declared)
		sort.Strings(sent)
		if !slices.Equal(declared, sent) {
			t.Errorf("%s: taxonomy dimensions %v, console.go allows %v", action, declared, sent)
		}
	}
}

// emittedEvent is one growthanalytics.Track call found in api/, with its
// dimensions resolved to literals. A value of "" means the code computes it
// at request time (e.g. DynamicOutcome), so only the key can be checked.
type emittedEvent struct {
	code string
	pos  string
	dims map[string]string
}

// TestTaxonomyDeclaresTrackedEvents parses the route registrations in api/
// and checks every Track call against the taxonomy: the code is declared,
// each dimension key is declared, each literal value is in the enum, and
// every required dimension is sent.
func TestTaxonomyDeclaresTrackedEvents(t *testing.T) {
	features := allFeatures(t, loadTaxonomy(t))
	events := findTrackCalls(t)
	if len(events) == 0 {
		t.Fatalf("found no growthanalytics.Track calls under %s; the parser is out of date", apiRoutesDir)
	}

	for _, ev := range events {
		f, ok := features[ev.code]
		if !ok {
			t.Errorf("%s: %q is emitted but not declared in %s", ev.pos, ev.code, taxonomyPath)
			continue
		}
		for key, value := range ev.dims {
			d, ok := f.dimension(key)
			if !ok {
				t.Errorf("%s: %s sends dimension %q, which the taxonomy does not declare", ev.pos, ev.code, key)
				continue
			}
			if value != "" && d.Type == "enum" && !slices.Contains(d.Values, value) {
				t.Errorf("%s: %s sends %s=%q, which is not in %v", ev.pos, ev.code, key, value, d.Values)
			}
		}
		for _, d := range f.Tracking.Dimensions {
			if _, sent := ev.dims[d.Name]; d.Required && !sent {
				t.Errorf("%s: %s does not send required dimension %q", ev.pos, ev.code, d.Name)
			}
		}
	}
}

// These features deliberately have no emitter yet. Keep this list explicit:
// YAML comments are documentation, not an exemption from coverage checks.
var notEmittedFeatures = map[string]bool{
	"amp.deployment-ops.deploy-agent.suspend":                     true,
	"amp.deployment-ops.create-environment.thunder-system-client": true,
	"amp.deployment-ops.a2a-proxy":                                true,
	"amp.deployment-ops.agent-card":                               true,
	"amp.connections.create-llm-proxy":                            true,
	"amp.connections.deploy-llm-proxy":                            true,
	"amp.connections.llm-proxy-api-key":                           true,
	"amp.connections.project-mcp-proxy":                           true,
	"amp.source-control.browse-repository":                        true,
	"amp.discovery.view-catalog":                                  true,
	"amp.discovery.view-build-options":                            true,
	"amp.observability.create-monitor.alerting":                   true,
	"amp.observability.view-scores":                               true,
	"amp.events.manage-destination":                               true,
	"amp.events.manage-subscription":                              true,
}

func TestTaxonomyFeaturesHaveEmitters(t *testing.T) {
	features := allFeatures(t, loadTaxonomy(t))
	emitted := map[string]bool{}
	for _, ev := range findTrackCalls(t) {
		emitted[ev.code] = true
	}
	for code := range consoleActions {
		emitted[code] = true
	}
	for code, f := range features {
		if !emitted[code] && !notEmittedFeatures[code] && f.Status != "deprecated" {
			t.Errorf("%s: declared feature has no emitter; add tracking or explicitly mark it not emitted", code)
		}
	}
	for code := range notEmittedFeatures {
		if _, declared := features[code]; !declared {
			t.Errorf("%s: not-emitted exemption has no taxonomy declaration", code)
		}
		if emitted[code] {
			t.Errorf("%s: now has an emitter; remove its not-emitted exemption and YAML comment", code)
		}
	}
}

// findTrackCalls returns every growthanalytics.Track(code, dims, handler)
// call in the api package. dims is either a map literal or a call to a
// package-level helper that builds one; helpers are resolved by substituting
// their string-literal arguments for the parameters they use.
func findTrackCalls(t *testing.T) []emittedEvent {
	t.Helper()
	handlerDims := findControllerDimensions(t)
	paths, err := filepath.Glob(filepath.Join(apiRoutesDir, "*.go"))
	if err != nil {
		t.Fatalf("list %s: %v", apiRoutesDir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files = append(files, file)
	}

	helpers := map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				helpers[fn.Name.Name] = fn
			}
		}
	}

	var events []emittedEvent
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isGrowthAnalyticsTrack(call) || len(call.Args) != 3 {
				return true
			}
			pos := fset.Position(call.Pos())
			at := filepath.Base(pos.Filename) + ":" + strconv.Itoa(pos.Line)
			code, ok := stringLiteral(call.Args[0])
			if !ok {
				t.Errorf("%s: Track code must be a string literal", at)
				return true
			}
			dims, ok := resolveDims(call.Args[1], helpers)
			if !ok {
				t.Errorf("%s: cannot resolve the dimensions passed to Track(%q)", at, code)
				return true
			}
			handler, ok := call.Args[2].(*ast.SelectorExpr)
			if !ok {
				t.Errorf("%s: cannot resolve the handler passed to Track(%q)", at, code)
				return true
			}
			controllerDims, found := handlerDims[handler.Sel.Name]
			if !found {
				t.Errorf("%s: controller method %s was not found", at, handler.Sel.Name)
				return true
			}
			for key, value := range controllerDims {
				dims[key] = value
			}
			events = append(events, emittedEvent{code: code, pos: at, dims: dims})
			return true
		})
	}
	return events
}

// Route registrations reference controller methods directly. Resolve their
// SetDimension keys too, including conditional calls. Runtime values remain
// unknown, just like computed values in a route's static dimension map.
func findControllerDimensions(t *testing.T) map[string]map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(controllersDir, "*.go"))
	if err != nil {
		t.Fatalf("list %s: %v", controllersDir, err)
	}
	fset := token.NewFileSet()
	out := map[string]map[string]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			dims := map[string]string{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "SetDimension" {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "growthanalytics" {
					return true
				}
				if len(call.Args) != 3 {
					t.Fatalf("%s: unexpected SetDimension arguments", fset.Position(call.Pos()))
				}
				key, ok := stringLiteral(call.Args[1])
				if !ok {
					t.Errorf("%s: SetDimension key must be a string literal", fset.Position(call.Pos()))
					return true
				}
				value, _ := stringLiteral(call.Args[2])
				dims[key] = value
				return true
			})
			// Shared method names with no dimensions are equivalent for this
			// check. Refuse to guess if either implementation sets dimensions.
			if previous, exists := out[fn.Name.Name]; exists && (len(previous) > 0 || len(dims) > 0) {
				t.Fatalf("controller method %s has ambiguous dimensions; resolve Track handlers by receiver type", fn.Name.Name)
			}
			out[fn.Name.Name] = dims
		}
	}
	return out
}

func isGrowthAnalyticsTrack(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Track" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "growthanalytics"
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// resolveDims turns Track's dimensions argument into key → literal value.
func resolveDims(expr ast.Expr, helpers map[string]*ast.FuncDecl) (map[string]string, bool) {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return collectDims(e, nil), true
	case *ast.CallExpr:
		ident, ok := e.Fun.(*ast.Ident)
		if !ok {
			return nil, false
		}
		fn, ok := helpers[ident.Name]
		if !ok {
			return nil, false
		}
		params := map[string]string{}
		i := 0
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				if i < len(e.Args) {
					if v, ok := stringLiteral(e.Args[i]); ok {
						params[name.Name] = v
					}
				}
				i++
			}
		}
		return collectDims(fn.Body, params), true
	default:
		return nil, false
	}
}

// collectDims gathers string keys from map literals (`"k": v`) and index
// assignments (`m["k"] = v`) under node. A parameter bound to "" means the
// helper's caller opted out of that key, so it is left out.
func collectDims(node ast.Node, params map[string]string) map[string]string {
	dims := map[string]string{}
	add := func(keyExpr, valueExpr ast.Expr) {
		key, ok := stringLiteral(keyExpr)
		if !ok {
			return
		}
		if v, ok := stringLiteral(valueExpr); ok {
			dims[key] = v
			return
		}
		if ident, ok := valueExpr.(*ast.Ident); ok {
			if v, bound := params[ident.Name]; bound {
				if v != "" {
					dims[key] = v
				}
				return
			}
		}
		dims[key] = ""
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			add(x.Key, x.Value)
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				if idx, ok := lhs.(*ast.IndexExpr); ok && i < len(x.Rhs) {
					add(idx.Index, x.Rhs[i])
				}
			}
		}
		return true
	})
	return dims
}
