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

package shadowagents

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Inventory file formats (Bring your own inventory).
const (
	InventoryFormatJSON = "json"
	InventoryFormatCSV  = "csv"
	InventoryFormatXLSX = "xlsx"
)

// Limits of one inventory file.
const (
	// MaxInventoryFileBytes is the largest file accepted (5 MB).
	MaxInventoryFileBytes = 5 << 20
	// MaxInventoryAgents is the most agent rows or objects a file may have.
	// A file is saved in one transaction within the request: 5000 agents
	// take a few seconds.
	MaxInventoryAgents = 5000
	// MaxInventoryProblems is how many problems are reported, the first ones.
	MaxInventoryProblems = 50
	// maxInventoryIgnored is how many unknown columns are reported.
	maxInventoryIgnored = 100
	// maxInventoryID is the longest id, in characters.
	maxInventoryID = 256
	// maxXLSXUnzipBytes bounds what an Excel file may unpack to in all, and
	// maxXLSXPartBytes a sheet or the shared strings, against zip bombs:
	// excelize holds the parts in memory (no temporary files at these sizes)
	// and decodes the shared strings, the styles and other small parts into
	// up to 50 times their size. Every other XML part may be at most
	// maxXLSXOtherXMLBytes, and maxXLSXOtherXMLTotal together.
	maxXLSXUnzipBytes    = 20 << 20
	maxXLSXPartBytes     = 8 << 20
	maxXLSXOtherXMLBytes = 1 << 20
	maxXLSXOtherXMLTotal = 2 << 20
	// maxXLSXStrings bounds the shared strings (a 5000-row, 14-column file
	// has at most 70000), and maxXLSXStringTags the elements of the shared
	// strings part, which rich text multiplies.
	maxXLSXStrings    = 150000
	maxXLSXStringTags = 1000000
	// maxXLSXParts bounds the parts of an Excel file; a workbook has a few
	// dozen.
	maxXLSXParts = 1000
	// maxXLSXColumns is the widest sheet read: excelize reads a row as one
	// value per column up to its last value, so a value in column XFD makes
	// every row 16384 values.
	maxXLSXColumns = 200
	// maxXLSXParses is how many Excel files are read at once.
	maxXLSXParses = 2
	// maxQuoted is how much of a value from the file a problem quotes.
	maxQuoted = 60
	// maxProblemMessage bounds a problem's message, in characters.
	maxProblemMessage = 300
	// maxQuotedLabels is how many bad labels a problem quotes.
	maxQuotedLabels = 3
)

// Inventory fields: the exact column names (CSV and Excel) and JSON keys.
const (
	InventoryFieldID          = "id"
	InventoryFieldName        = "name"
	InventoryFieldDescription = "description"
	InventoryFieldSource      = "source"
	InventoryFieldURL         = "url"
	InventoryFieldFramework   = "framework"
	InventoryFieldModel       = "model"
	InventoryFieldVersion     = "version"
	InventoryFieldStatus      = "status"
	InventoryFieldScope       = "scope"
	InventoryFieldOwnerName   = "ownerName"
	InventoryFieldOwnerEmail  = "ownerEmail"
	InventoryFieldCreatedAt   = "createdAt"
	InventoryFieldLabels      = "labels"
)

// InventoryField is one field of the inventory file format. Example is
// written as in a CSV or Excel cell.
type InventoryField struct {
	Name        string
	Required    bool
	Description string
	Example     string
}

// inventoryFields is the format, in column order.
var inventoryFields = []InventoryField{
	{
		Name: InventoryFieldID, Required: true, Example: "support-bot-prod",
		Description: "A unique ID for the agent in your inventory. The next upload matches agents by this ID.",
	},
	{Name: InventoryFieldName, Required: true, Example: "Customer support bot", Description: "The agent's name."},
	{
		Name: InventoryFieldDescription, Example: "Answers customer questions in the help center.",
		Description: "What the agent does.",
	},
	{
		Name: InventoryFieldSource, Example: "Internal platform",
		Description: "Where the agent runs or was built, such as a platform or a team's service.",
	},
	{
		Name: InventoryFieldURL, Example: "https://agents.example.com/support-bot",
		Description: "An http or https link to the agent. Only the scheme, host and path are kept.",
	},
	{Name: InventoryFieldFramework, Example: "LangGraph", Description: "The agent framework, such as LangGraph or CrewAI."},
	{Name: InventoryFieldModel, Example: "gpt-4o", Description: "The model the agent uses."},
	{Name: InventoryFieldVersion, Example: "1.4.0", Description: "The agent's version."},
	{Name: InventoryFieldStatus, Example: "active", Description: "The agent's status in your inventory, such as active or inactive."},
	{
		Name: InventoryFieldScope, Example: "production",
		Description: "Where the agent belongs, such as an environment, a team or a project.",
	},
	{Name: InventoryFieldOwnerName, Example: "Jane Perera", Description: "The name of the agent's owner."},
	{Name: InventoryFieldOwnerEmail, Example: "jane.perera@example.com", Description: "The owner's email address."},
	{
		Name: InventoryFieldCreatedAt, Example: "2026-01-15T09:30:00Z",
		Description: "When the agent was created: a date (2026-01-15) or a date and time (2026-01-15T09:30:00Z).",
	},
	{
		Name: InventoryFieldLabels, Example: "team=support; region=eu",
		Description: "Extra key=value pairs separated by semicolons. In JSON, an object of text values.",
	},
}

// inventorySecondExample is the template's second example agent, in
// inventoryFields order.
var inventorySecondExample = []string{
	"hr-assistant", "HR policy assistant", "Answers employee questions about HR policies.",
	"Microsoft Copilot Studio", "https://agents.example.com/hr-assistant", "Copilot Studio", "gpt-4o-mini",
	"2.0.3", "active", "hr", "Arjun Silva", "arjun.silva@example.com", "2025-11-03", "team=hr",
}

// InventoryFields returns the inventory file format, in column order.
func InventoryFields() []InventoryField {
	return append([]InventoryField{}, inventoryFields...)
}

// inventoryHeaders maps a normalised column name or JSON key to its field.
var inventoryHeaders = func() map[string]string {
	out := make(map[string]string, len(inventoryFields))
	for _, f := range inventoryFields {
		out[normaliseInventoryHeader(f.Name)] = f.Name
	}
	return out
}()

// normaliseInventoryHeader matches column names case-insensitively,
// ignoring spaces, "_" and "-": "Owner Email" and "owner_email" are
// ownerEmail.
func normaliseInventoryHeader(h string) string {
	h = strings.TrimPrefix(strings.TrimSpace(h), "\ufeff")
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "", "\t", "").Replace(h))
}

// InventoryFileError is a file that cannot be used: nothing in it is saved.
// Its message is plain and safe to show to the user.
type InventoryFileError struct {
	Message string
}

func (e *InventoryFileError) Error() string { return e.Message }

func inventoryFileErr(format string, a ...any) error {
	return &InventoryFileError{Message: fmt.Sprintf(format, a...)}
}

// InventoryProblem is a problem with one agent row (CSV, Excel: the
// spreadsheet row number, the header being row 1) or object (JSON: its
// 1-based position in the agents list). LeftOut says the agent was left out;
// otherwise it was kept without the value the message names.
type InventoryProblem struct {
	Row     int
	Message string
	LeftOut bool
}

// InventoryFile is what an inventory file lists. Findings are the agents to
// save, one per distinct id, already sanitised; their evidence names the
// upload (InventoryCallName).
type InventoryFile struct {
	Format string
	// Findings are the valid agents, in file order.
	Findings []*Finding
	// AgentsRead counts the agent rows or objects in the file.
	AgentsRead int
	// LeftOut counts the rows or objects left out: invalid, or repeating an
	// id listed earlier.
	LeftOut int
	// Problems are the first MaxInventoryProblems problems, in file order.
	Problems []InventoryProblem
	// ProblemCount counts every problem found, before the cap.
	ProblemCount int
	// IgnoredColumns are the columns or keys not in the format, sorted.
	IgnoredColumns []string
}

// InventoryFormatLabel names a format for people: JSON, CSV or Excel.
func InventoryFormatLabel(format string) string {
	switch format {
	case InventoryFormatJSON:
		return "JSON"
	case InventoryFormatCSV:
		return "CSV"
	case InventoryFormatXLSX:
		return "Excel"
	}
	return "inventory"
}

// InventoryCallName names an upload's call result and its agents' evidence
// source, e.g. "Uploaded CSV file". The user's file name is never part of
// it.
func InventoryCallName(format string) string {
	return "Uploaded " + InventoryFormatLabel(format) + " file"
}

// ParseInventoryFile reads an inventory file. name is the uploaded file's
// name, used only for its extension. A file that cannot be used at all
// fails with an *InventoryFileError; a bad row is a problem, not a failed
// file.
func ParseInventoryFile(name string, data []byte) (*InventoryFile, error) {
	if len(data) == 0 {
		return nil, inventoryFileErr("The file is empty. Download a template, fill in your agents and upload it.")
	}
	if len(data) > MaxInventoryFileBytes {
		return nil, inventoryFileErr("The file is larger than 5 MB. Remove the columns that are not in the format, then upload it again.")
	}
	format, err := detectInventoryFormat(name, data)
	if err != nil {
		return nil, err
	}
	p := &inventoryParser{
		out:     &InventoryFile{Format: format, Findings: []*Finding{}, Problems: []InventoryProblem{}},
		call:    InventoryCallName(format),
		seen:    map[string]bool{},
		ignored: map[string]bool{},
	}
	switch format {
	case InventoryFormatJSON:
		err = p.parseJSON(data)
	case InventoryFormatCSV:
		err = p.parseCSV(data)
	default:
		err = p.parseXLSX(data)
	}
	if err != nil {
		return nil, err
	}
	out := p.out
	if out.AgentsRead > 0 && len(out.Findings) == 0 {
		first := ""
		if len(out.Problems) > 0 {
			first = fmt.Sprintf(" Row %d: %s", out.Problems[0].Row, out.Problems[0].Message)
		}
		return nil, inventoryFileErr("None of the %d agents in the file can be used: each needs an id and a name, "+
			"and an id can be listed only once.%s", out.AgentsRead, first)
	}
	out.IgnoredColumns = make([]string, 0, len(p.ignored))
	for c := range p.ignored {
		out.IgnoredColumns = append(out.IgnoredColumns, c)
	}
	sort.Strings(out.IgnoredColumns)
	if len(out.IgnoredColumns) > maxInventoryIgnored {
		out.IgnoredColumns = out.IgnoredColumns[:maxInventoryIgnored]
	}
	return out, nil
}

var (
	utf8BOM   = []byte("\ufeff")
	oleHeader = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

const msgTooManyAgents = "The file has more than 5000 agents. At most 5000 agents can be uploaded in one file."

const msgLegacyXLS = "Excel 97-2003 workbooks (.xls) are not supported. Open the file in Excel and save it as an " +
	"Excel workbook (.xlsx), or as CSV UTF-8."

// detectInventoryFormat reads the format from the file name's extension,
// or else from the content: a ZIP file is an Excel workbook, and text that
// starts with { or [ is JSON.
func detectInventoryFormat(name string, data []byte) (string, error) {
	switch strings.ToLower(path.Ext(strings.TrimSpace(name))) {
	case ".json":
		return InventoryFormatJSON, nil
	case ".csv":
		return InventoryFormatCSV, nil
	case ".xlsx":
		if bytes.HasPrefix(data, oleHeader) {
			// A password-protected workbook is stored in the old container.
			return "", inventoryFileErr("The Excel file is password-protected or in the Excel 97-2003 format. " +
				"Save it as an Excel workbook (.xlsx) without a password.")
		}
		return InventoryFormatXLSX, nil
	case ".xls":
		return "", inventoryFileErr(msgLegacyXLS)
	}
	switch {
	case bytes.HasPrefix(data, []byte("PK")):
		return InventoryFormatXLSX, nil
	case bytes.HasPrefix(data, oleHeader):
		return "", inventoryFileErr(msgLegacyXLS)
	}
	text := bytes.TrimLeft(bytes.TrimPrefix(data, utf8BOM), " \t\r\n")
	if len(text) > 0 && (text[0] == '{' || text[0] == '[') {
		return InventoryFormatJSON, nil
	}
	return "", inventoryFileErr("This file type is not supported. Upload a .json, .csv or .xlsx file.")
}

// inventoryParser collects the agents of one file.
type inventoryParser struct {
	out     *InventoryFile
	call    string
	seen    map[string]bool
	ignored map[string]bool
}

// problem records a problem with a kept agent.
func (p *inventoryParser) problem(row int, format string, a ...any) {
	p.record(row, false, format, a...)
}

// leaveOut records a problem that left the agent out.
func (p *inventoryParser) leaveOut(row int, format string, a ...any) {
	p.out.LeftOut++
	p.record(row, true, format, a...)
}

func (p *inventoryParser) record(row int, leftOut bool, format string, a ...any) {
	p.out.ProblemCount++
	if len(p.out.Problems) < MaxInventoryProblems {
		msg := Truncate(fmt.Sprintf(format, a...), maxProblemMessage)
		p.out.Problems = append(p.out.Problems, InventoryProblem{Row: row, Message: msg, LeftOut: leftOut})
	}
}

// cleanText drops NUL and the other control characters except tab and new
// line, which the database cannot store or no field needs, and trims it.
func cleanText(s string) string {
	if strings.IndexFunc(s, isDroppedControl) >= 0 {
		s = strings.Map(func(r rune) rune {
			if isDroppedControl(r) {
				return -1
			}
			return r
		}, s)
	}
	return strings.TrimSpace(s)
}

func isDroppedControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n') || r == 0x7f
}

func (p *inventoryParser) ignore(column string) {
	if c := Truncate(cleanText(strings.TrimPrefix(column, "\ufeff")), 100); c != "" {
		p.ignored[c] = true
	}
}

// countAgent counts one more agent row, failing once the file has too many.
func (p *inventoryParser) countAgent() error {
	p.out.AgentsRead++
	if p.out.AgentsRead > MaxInventoryAgents {
		return inventoryFileErr(msgTooManyAgents)
	}
	return nil
}

// quote quotes a value from the file in a problem, shortened.
func quote(s string) string {
	return strconv.Quote(Truncate(s, maxQuoted))
}

// inventoryRow is one agent row's values by field, with its labels apart.
type inventoryRow struct {
	values map[string]string
	labels map[string]string
	// badLabels are the label entries that are not key=value.
	badLabels []string
	// warnings are problems with values of the row found while reading it,
	// recorded only when the agent is kept.
	warnings []string
}

func (r *inventoryRow) warn(format string, a ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, a...))
}

// add turns one row into an agent, or leaves it out. The row's warnings
// and the problems with its values are recorded only when it is kept.
func (p *inventoryParser) add(row int, r inventoryRow, allowSerialDate bool) {
	v := r.values
	id, name := v[InventoryFieldID], v[InventoryFieldName]
	switch {
	case id == "" && name == "":
		p.leaveOut(row, "No id and no name; the agent was left out.")
		return
	case id == "":
		p.leaveOut(row, "No id; the agent was left out.")
		return
	case name == "":
		p.leaveOut(row, "No name; the agent was left out.")
		return
	case utf8.RuneCountInString(id) > maxInventoryID:
		p.leaveOut(row, "The id is longer than %d characters; the agent was left out.", maxInventoryID)
		return
	case p.seen[id]:
		p.leaveOut(row, "The id %s is listed in an earlier row; this row was left out.", quote(id))
		return
	}
	p.seen[id] = true
	for _, w := range r.warnings {
		p.problem(row, "%s", w)
	}
	f := &Finding{
		Kind: models.ShadowKindPlatformAgent, ExternalID: id, DisplayName: SanitizeName(name),
		Scope: v[InventoryFieldScope], State: v[InventoryFieldStatus], Attributes: map[string]any{},
		OwnerHint: MinimiseOwnerHint(models.ShadowOwnerHint{Name: v[InventoryFieldOwnerName], Email: v[InventoryFieldOwnerEmail]}),
	}
	if raw := v[InventoryFieldCreatedAt]; raw != "" {
		if t, ok := parseInventoryDate(raw, allowSerialDate); ok {
			f.ProviderCreatedAt = &t
		} else {
			p.problem(row, "createdAt %s is not a date such as 2026-01-15 or 2026-01-15T09:30:00Z; the date was left out.",
				quote(raw))
		}
	}
	if raw := v[InventoryFieldURL]; raw != "" {
		if u := RepositoryURL(raw); u != "" {
			f.Attributes["agentUrl"] = u
		} else {
			p.problem(row, "url %s is not an http or https link; the link was left out.", quote(raw))
		}
	}
	for field, attr := range map[string]string{
		InventoryFieldDescription: "description", InventoryFieldSource: "source", InventoryFieldFramework: "framework",
		InventoryFieldModel: "model", InventoryFieldVersion: "agentVersion",
	} {
		if s := v[field]; s != "" {
			f.Attributes[attr] = s
		}
	}
	if len(r.badLabels) > 0 {
		quoted := make([]string, 0, maxQuotedLabels)
		for _, b := range r.badLabels[:min(len(r.badLabels), maxQuotedLabels)] {
			quoted = append(quoted, quote(b))
		}
		more := ""
		if n := len(r.badLabels) - len(quoted); n > 0 {
			more = fmt.Sprintf(" and %d more", n)
		}
		p.problem(row, "labels %s%s are not key=value pairs; they were left out.", strings.Join(quoted, ", "), more)
	}
	if len(r.labels) > 0 {
		f.Attributes["labels"] = r.labels
	}
	f.Attributes = SanitizeAttributes(f.Attributes)
	f.AddSignal(Signal{Code: SigCustomAgent, Source: p.call})
	p.out.Findings = append(p.out.Findings, f)
}

// parseLabelText reads labels written as key=value pairs separated by ";" or
// new lines. It returns the labels and the entries that are not key=value.
func parseLabelText(s string) (map[string]string, []string) {
	labels := map[string]string{}
	var bad []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, val, ok := strings.Cut(part, "=")
		k, val = strings.TrimSpace(k), strings.TrimSpace(val)
		if !ok || k == "" {
			bad = append(bad, part)
			continue
		}
		labels[k] = val
	}
	return labels, bad
}

// inventoryDateLayouts are the dates createdAt accepts; a layout without a
// zone is UTC.
var inventoryDateLayouts = []string{
	time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02",
}

// parseInventoryDate reads createdAt. An Excel date shown as a number (a
// date cell formatted as General) is read as an Excel date when
// allowSerial.
func parseInventoryDate(s string, allowSerial bool) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range inventoryDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	if allowSerial {
		if n, err := strconv.ParseFloat(s, 64); err == nil && n >= 1 && n < 2958466 {
			if t, err := excelize.ExcelDateToTime(n, false); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

// ---- JSON ----

func (p *inventoryParser) parseJSON(data []byte) error {
	data = bytes.TrimPrefix(data, utf8BOM)
	dec := json.NewDecoder(bytes.NewReader(data))
	var top json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return inventoryFileErr("The file is not valid JSON%s. Fix it, or start from the JSON template.",
			jsonErrorPlace(err))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return inventoryFileErr("The file is not valid JSON: there is more data after the agents.")
	}
	var items []json.RawMessage
	switch bytes.TrimLeft(top, " \t\r\n")[0] {
	case '[':
		if err := json.Unmarshal(top, &items); err != nil {
			return inventoryFileErr("The file is not valid JSON.")
		}
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(top, &obj); err != nil {
			return inventoryFileErr("The file is not valid JSON.")
		}
		raw, ok := obj["agents"]
		if !ok {
			return inventoryFileErr(`The JSON file has no "agents" list. Use {"agents": [...]}, or a list of agents.`)
		}
		if err := json.Unmarshal(raw, &items); err != nil || items == nil {
			return inventoryFileErr(`"agents" must be a list of agents.`)
		}
	default:
		return inventoryFileErr(`The JSON file must be an object with an "agents" list, or a list of agents.`)
	}
	if len(items) > MaxInventoryAgents {
		return inventoryFileErr(msgTooManyAgents)
	}
	for i, item := range items {
		if err := p.countAgent(); err != nil {
			return err
		}
		p.jsonAgent(i+1, item)
	}
	return nil
}

// jsonErrorPlace says where a JSON syntax error is, when the decoder knows.
func jsonErrorPlace(err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf(" (at character %d)", syntax.Offset)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return ": it ends too early"
	}
	return ""
}

// jsonAgent reads one agent object. Each is decoded on its own, so an
// object that does not match the format is a problem, not a failed file.
func (p *inventoryParser) jsonAgent(row int, item json.RawMessage) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(item, &obj); err != nil || obj == nil {
		p.leaveOut(row, "Not an object with an id and a name; the agent was left out.")
		return
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	r := inventoryRow{values: map[string]string{}}
	labelsSet := false
	for _, k := range keys {
		field, known := inventoryHeaders[normaliseInventoryHeader(k)]
		if !known {
			p.ignore(k)
			continue
		}
		if _, dup := r.values[field]; dup || (field == InventoryFieldLabels && labelsSet) {
			p.ignore(k)
			continue
		}
		raw := obj[k]
		if field == InventoryFieldLabels {
			labelsSet = true
			labels, bad, ok := jsonLabels(raw)
			if !ok {
				r.warn("labels must be an object of text values; they were left out.")
				continue
			}
			r.labels, r.badLabels = labels, bad
			continue
		}
		s, ok := jsonText(raw)
		if !ok {
			if field == InventoryFieldID || field == InventoryFieldName {
				p.leaveOut(row, "%s must be text; the agent was left out.", field)
				return
			}
			r.warn("%s must be text; it was left out.", field)
			continue
		}
		r.values[field] = s
	}
	p.add(row, r, false)
}

// jsonText reads a text value: a string, or a number or boolean as written.
// null is empty. Objects and lists are not text.
func jsonText(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", true
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", false
		}
		return cleanText(s), true
	case '{', '[':
		return "", false
	default:
		// A number, true or false, as written.
		return string(raw), true
	}
}

// jsonLabels reads labels: an object of text values, or key=value text.
// Entries that are not text are returned as bad.
func jsonLabels(raw json.RawMessage) (map[string]string, []string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, true
	}
	switch raw[0] {
	case '"':
		s, _ := jsonText(raw)
		labels, bad := parseLabelText(s)
		return labels, bad, true
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, nil, false
		}
		labels := map[string]string{}
		var bad []string
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, ok := jsonText(obj[k])
			key := cleanText(k)
			if !ok || key == "" {
				bad = append(bad, k)
				continue
			}
			labels[key] = v
		}
		return labels, bad, true
	default:
		return nil, nil, false
	}
}

// ---- CSV and Excel ----

// columns maps a header row to the field of each column ("" for a column
// not in the format, which is reported as ignored). The id and name columns
// are required.
func (p *inventoryParser) columns(header []string) ([]string, error) {
	cols := make([]string, len(header))
	used := map[string]bool{}
	for i, h := range header {
		field, known := inventoryHeaders[normaliseInventoryHeader(h)]
		if !known || used[field] {
			p.ignore(h)
			continue
		}
		used[field] = true
		cols[i] = field
	}
	for _, req := range []string{InventoryFieldID, InventoryFieldName} {
		if !used[req] {
			return nil, inventoryFileErr("The file has no %s column. The first row of the file must hold the "+
				"column names, such as id and name; download the template to see them.", req)
		}
	}
	return cols, nil
}

// row adds one CSV or Excel row; a row with no value is skipped.
func (p *inventoryParser) row(rowNum int, cols, cells []string, allowSerialDate bool) error {
	r := inventoryRow{values: map[string]string{}}
	empty := true
	for i, c := range cells {
		c = cleanText(c)
		if c == "" {
			continue
		}
		empty = false
		if i >= len(cols) || cols[i] == "" {
			continue
		}
		if cols[i] == InventoryFieldLabels {
			r.labels, r.badLabels = parseLabelText(c)
			continue
		}
		r.values[cols[i]] = c
	}
	if empty {
		return nil
	}
	if err := p.countAgent(); err != nil {
		return err
	}
	p.add(rowNum, r, allowSerialDate)
	return nil
}

func (p *inventoryParser) parseCSV(data []byte) error {
	data = bytes.TrimPrefix(data, utf8BOM)
	if !utf8.Valid(data) {
		return inventoryFileErr("The CSV file is not UTF-8 text. Save it as CSV UTF-8 (in Excel: Save As, " +
			"then CSV UTF-8) and upload it again.")
	}
	r := csv.NewReader(bytes.NewReader(data))
	// Excel writes ";" between values in locales that use "," in numbers.
	first, _, _ := bytes.Cut(data, []byte("\n"))
	if !bytes.Contains(first, []byte(",")) && bytes.Contains(first, []byte(";")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return inventoryFileErr("The file is empty. Download a template, fill in your agents and upload it.")
	}
	if err != nil {
		return inventoryFileErr("The CSV file cannot be read: %s.", csvErrorMessage(err))
	}
	cols, err := p.columns(header)
	if err != nil {
		return err
	}
	// The spreadsheet row of a record: the reader skips blank lines, which a
	// spreadsheet shows as rows, and a quoted value can span lines, which a
	// spreadsheet shows in one row.
	headerLine, _ := r.FieldPos(0)
	rowNum, prevEnd := headerLine, csvRecordEnd(r, header)
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return inventoryFileErr("The CSV file cannot be read: %s.", csvErrorMessage(err))
		}
		start, _ := r.FieldPos(0)
		rowNum += 1 + max(start-prevEnd-1, 0)
		prevEnd = csvRecordEnd(r, rec)
		if err := p.row(rowNum, cols, rec, false); err != nil {
			return err
		}
	}
}

// csvRecordEnd is the line the record just read ends on.
func csvRecordEnd(r *csv.Reader, rec []string) int {
	last := len(rec) - 1
	line, _ := r.FieldPos(last)
	return line + strings.Count(rec[last], "\n")
}

func csvErrorMessage(err error) string {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return fmt.Sprintf("line %d: %s", pe.Line, pe.Err)
	}
	return "it is not valid CSV"
}

// xlsxParses holds a slot per Excel file being read.
var xlsxParses = make(chan struct{}, maxXLSXParses)

const msgXLSXInvalid = "The file is not a valid Excel workbook. Save it as an Excel workbook (.xlsx), " +
	"then upload it again."

const msgXLSXTooManyStrings = "The Excel file holds too many different text values to read. Remove the " +
	"columns that are not in the format, or save it as CSV UTF-8, then upload it again."

const msgXLSXTooLarge = "The Excel file unpacks to too much data to read. Remove the columns that are not in " +
	"the format, or save it as CSV UTF-8, then upload it again."

// checkXLSXSize refuses an Excel file that unpacks to more than excelize is
// allowed to hold, from the sizes its zip directory declares (the zip reader
// fails on a part longer than declared).
func checkXLSXSize(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return inventoryFileErr(msgXLSXInvalid)
	}
	if len(zr.File) > maxXLSXParts {
		return inventoryFileErr(msgXLSXInvalid)
	}
	var total, otherXML uint64
	for _, zf := range zr.File {
		size := zf.UncompressedSize64
		// Part names as excelize reads them.
		name := strings.ToLower(strings.ReplaceAll(zf.Name, "\\", "/"))
		switch {
		case name == "xl/sharedstrings.xml":
			if size > maxXLSXPartBytes {
				return inventoryFileErr(msgXLSXTooLarge)
			}
			if err := checkXLSXStrings(zf); err != nil {
				return err
			}
		case strings.HasPrefix(name, "xl/worksheets/"):
			// Sheets are read one row at a time, and only the first.
			if size > maxXLSXPartBytes {
				return inventoryFileErr(msgXLSXTooLarge)
			}
		case strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") || strings.HasSuffix(name, ".vml"):
			// Styles, the workbook, relationships, content types and the
			// like, which excelize decodes whole.
			if size > maxXLSXOtherXMLBytes {
				return inventoryFileErr(msgXLSXTooLarge)
			}
			otherXML += size
		}
		// Pictures and other files are held, never decoded.
		total += size
	}
	if total > maxXLSXUnzipBytes || otherXML > maxXLSXOtherXMLTotal {
		return inventoryFileErr(msgXLSXTooLarge)
	}
	return nil
}

// checkXLSXStrings refuses shared strings that excelize would decode into
// too much memory: too many strings, or too many elements in them.
func checkXLSXStrings(zf *zip.File) error {
	rc, err := zf.Open()
	if err != nil {
		return inventoryFileErr(msgXLSXInvalid)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, maxXLSXPartBytes+1))
	if err != nil {
		return inventoryFileErr(msgXLSXInvalid)
	}
	if bytes.Count(b, []byte("<si")) > maxXLSXStrings || bytes.Count(b, []byte("<")) > maxXLSXStringTags {
		return inventoryFileErr(msgXLSXTooManyStrings)
	}
	return nil
}

func (p *inventoryParser) parseXLSX(data []byte) error {
	if err := checkXLSXSize(data); err != nil {
		return err
	}
	xlsxParses <- struct{}{}
	defer func() { <-xlsxParses }()
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    maxXLSXUnzipBytes,
		UnzipXMLSizeLimit: maxXLSXPartBytes,
		// Date cells in the default date formats read as 2026-01-15.
		ShortDatePattern: "yyyy-mm-dd",
	})
	if err != nil {
		if strings.Contains(err.Error(), "unzip size") {
			return inventoryFileErr(msgXLSXTooLarge)
		}
		return inventoryFileErr(msgXLSXInvalid)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return inventoryFileErr("The Excel workbook has no sheet.")
	}
	if col, row, wide := xlsxWideCell(f, xlsxFirstSheetParts(f)); wide {
		name, _ := excelize.ColumnNumberToName(col)
		return inventoryFileErr("The first sheet has a value in column %s (row %d). At most %d columns can be "+
			"read: delete the columns that are not in the format, then upload it again.", name, row, maxXLSXColumns)
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return inventoryFileErr("The first sheet of the Excel workbook cannot be read.")
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	// Next moves one spreadsheet row at a time, empty rows included, so the
	// count is the row number.
	for rowNum := 1; rows.Next(); rowNum++ {
		cells, err := rows.Columns()
		if err != nil {
			return inventoryFileErr("The first sheet of the Excel workbook cannot be read.")
		}
		if cols == nil {
			if cols, err = p.columns(cells); err != nil {
				return err
			}
			continue
		}
		if err := p.row(rowNum, cols, cells, true); err != nil {
			return err
		}
	}
	if rows.Error() != nil {
		return inventoryFileErr("The first sheet of the Excel workbook cannot be read.")
	}
	if cols == nil {
		return inventoryFileErr("The file is empty. Download a template, fill in your agents and upload it.")
	}
	return nil
}

// xlsxFirstSheetParts returns the part that holds the workbook's first
// sheet, found as excelize finds it. When it cannot be found that way, it
// returns every part, so the width check still covers the sheet excelize
// reads.
func xlsxFirstSheetParts(f *excelize.File) [][]byte {
	if part, ok := xlsxFirstSheetPart(f); ok {
		return [][]byte{part}
	}
	var parts [][]byte
	f.Pkg.Range(func(_, v any) bool {
		if b, ok := v.([]byte); ok {
			parts = append(parts, b)
		}
		return true
	})
	return parts
}

// xlsxRels is a relationships part.
type xlsxRels struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Type   string `xml:"Type,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func xlsxReadRels(f *excelize.File, name string) (xlsxRels, bool) {
	var rels xlsxRels
	v, ok := f.Pkg.Load(name)
	if !ok {
		return rels, false
	}
	b, ok := v.([]byte)
	if !ok {
		return rels, false
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = f.CharsetReader
	return rels, d.Decode(&rels) == nil
}

func xlsxFirstSheetPart(f *excelize.File) ([]byte, bool) {
	if f.WorkBook == nil || len(f.WorkBook.Sheets.Sheet) == 0 {
		return nil, false
	}
	// excelize finds a sheet by its name ignoring case: with two such names
	// it may read either.
	names := map[string]bool{}
	for _, sh := range f.WorkBook.Sheets.Sheet {
		key := strings.ToLower(sh.Name)
		if names[key] {
			return nil, false
		}
		names[key] = true
	}
	root, ok := xlsxReadRels(f, "_rels/.rels")
	if !ok {
		return nil, false
	}
	wbPath := ""
	for _, rel := range root.Rels {
		if rel.Type == excelize.SourceRelationshipOfficeDocument {
			wbPath = strings.TrimPrefix(rel.Target, "/")
			break
		}
	}
	if wbPath == "" {
		return nil, false
	}
	dir := path.Dir(wbPath)
	relsPath := strings.TrimPrefix(dir+"/_rels/"+path.Base(wbPath)+".rels", "./")
	rels, ok := xlsxReadRels(f, relsPath)
	if !ok {
		return nil, false
	}
	id := f.WorkBook.Sheets.Sheet[0].ID
	for _, rel := range rels.Rels {
		if rel.ID != id {
			continue
		}
		// As excelize's getWorksheetPath.
		target := strings.TrimPrefix(strings.ReplaceAll(path.Clean(dir+"/"+rel.Target), "\\", "/"), "/")
		if strings.HasPrefix(rel.Target, "/") {
			target = strings.TrimPrefix(strings.ReplaceAll(path.Clean(rel.Target), "\\", "/"), "/")
		}
		if v, ok := f.Pkg.Load(target); ok {
			b, ok := v.([]byte)
			return b, ok
		}
		return nil, false
	}
	return nil, false
}

// xlsxWideCell finds the first cell with a value (or formula) beyond
// maxXLSXColumns in the sheet parts, reading them as excelize does: an
// empty value, like an empty formatted cell, is not read.
func xlsxWideCell(f *excelize.File, parts [][]byte) (col, row int, wide bool) {
	for _, part := range parts {
		d := xml.NewDecoder(bytes.NewReader(part))
		d.CharsetReader = f.CharsetReader
		row, col = 0, 0
		inCell, inText := false, false
		for {
			tok, err := d.Token()
			if err != nil {
				// The end, or where excelize stops reading too.
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "row":
					row++
					col = 0
					if n, err := strconv.Atoi(xmlAttr(t, "r")); err == nil {
						row = n
					}
				case "c":
					col++
					if c, _, err := excelize.CellNameToCoordinates(xmlAttr(t, "r")); err == nil {
						col = c
					}
					inCell = true
				case "v", "t":
					inText = inCell
				case "f":
					if inCell && col > maxXLSXColumns {
						return col, row, true
					}
				}
			case xml.CharData:
				if inText && len(t) > 0 && col > maxXLSXColumns {
					return col, row, true
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "c":
					inCell, inText = false, false
				case "v", "t":
					inText = false
				}
			}
		}
	}
	return 0, 0, false
}

func xmlAttr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// ---- templates ----

// InventoryTemplateFileName is the template's download name.
func InventoryTemplateFileName(format string) string {
	return "agent-inventory-template." + format
}

// InventoryContentType is a format's media type.
func InventoryContentType(format string) string {
	switch format {
	case InventoryFormatJSON:
		return "application/json"
	case InventoryFormatCSV:
		return "text/csv; charset=utf-8"
	case InventoryFormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "application/octet-stream"
}

// IsInventoryFormat reports whether format is json, csv or xlsx.
func IsInventoryFormat(format string) bool {
	return format == InventoryFormatJSON || format == InventoryFormatCSV || format == InventoryFormatXLSX
}

// inventoryTemplateRows are the header and the two example agents.
func inventoryTemplateRows() [][]string {
	header := make([]string, 0, len(inventoryFields))
	first := make([]string, 0, len(inventoryFields))
	for _, f := range inventoryFields {
		header = append(header, f.Name)
		first = append(first, f.Example)
	}
	return [][]string{header, first, append([]string{}, inventorySecondExample...)}
}

// InventoryTemplate returns a template file in the format: the columns in
// order and two example agents.
func InventoryTemplate(format string) ([]byte, error) {
	rows := inventoryTemplateRows()
	switch format {
	case InventoryFormatJSON:
		return inventoryJSONTemplate(rows)
	case InventoryFormatCSV:
		var b bytes.Buffer
		w := csv.NewWriter(&b)
		if err := w.WriteAll(rows); err != nil {
			return nil, fmt.Errorf("writing the CSV template: %w", err)
		}
		return b.Bytes(), nil
	case InventoryFormatXLSX:
		return inventoryXLSXTemplate(rows)
	}
	return nil, fmt.Errorf("unknown inventory format %q", format)
}

// inventoryJSONTemplate writes {"agents": [...]} with the keys in format
// order, labels as an object.
func inventoryJSONTemplate(rows [][]string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"agents":[`)
	for i, ex := range rows[1:] {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('{')
		for j, name := range rows[0] {
			if j > 0 {
				b.WriteByte(',')
			}
			var value any = ex[j]
			if name == InventoryFieldLabels {
				value, _ = parseLabelText(ex[j])
			}
			k, err := json.Marshal(name)
			if err != nil {
				return nil, err
			}
			v, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			b.Write(k)
			b.WriteByte(':')
			b.Write(v)
		}
		b.WriteByte('}')
	}
	b.WriteString(`]}`)
	var out bytes.Buffer
	if err := json.Indent(&out, b.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("writing the JSON template: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// inventoryXLSXTemplate writes one "Agents" sheet, every cell as text.
func inventoryXLSXTemplate(rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Agents"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("naming the template sheet: %w", err)
	}
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				return nil, err
			}
			if err := f.SetCellStr(sheet, cell, v); err != nil {
				return nil, fmt.Errorf("writing the template: %w", err)
			}
		}
	}
	lastCol, err := excelize.ColumnNumberToName(len(rows[0]))
	if err != nil {
		return nil, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, fmt.Errorf("styling the template: %w", err)
	}
	if err := f.SetCellStyle(sheet, "A1", lastCol+"1", bold); err != nil {
		return nil, fmt.Errorf("styling the template: %w", err)
	}
	if err := f.SetColWidth(sheet, "A", lastCol, 24); err != nil {
		return nil, fmt.Errorf("styling the template: %w", err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("writing the Excel template: %w", err)
	}
	return buf.Bytes(), nil
}
