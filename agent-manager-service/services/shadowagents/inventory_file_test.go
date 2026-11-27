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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func parseOK(t *testing.T, name, data string) *InventoryFile {
	t.Helper()
	out, err := ParseInventoryFile(name, []byte(data))
	require.NoError(t, err)
	return out
}

func parseFails(t *testing.T, name string, data []byte, contains string) {
	t.Helper()
	_, err := ParseInventoryFile(name, data)
	var bad *InventoryFileError
	require.ErrorAs(t, err, &bad, name)
	assert.Contains(t, bad.Message, contains)
}

func findingByID(t *testing.T, f *InventoryFile, id string) *Finding {
	t.Helper()
	for _, x := range f.Findings {
		if x.ExternalID == id {
			return x
		}
	}
	t.Fatalf("no agent %q", id)
	return nil
}

// TestInventoryCSV reads every field of the format, with header aliases.
func TestInventoryCSV(t *testing.T) {
	data := "ID,Name,Description,Source,URL,Framework,Model,Version,Status,Scope,Owner Name,owner_email,created-at,Labels\n" +
		"support-bot-prod,Customer support bot,Answers questions.,Internal platform," +
		"https://user:pw@agents.example.com/support-bot?token=x#frag,LangGraph,gpt-4o,1.4.0,active,production," +
		"Jane Perera,jane.perera@example.com,2026-01-15T09:30:00Z,\"team=support; region=eu\"\n"
	out := parseOK(t, "inventory.CSV", data)
	assert.Equal(t, InventoryFormatCSV, out.Format)
	assert.Equal(t, 1, out.AgentsRead)
	assert.Zero(t, out.LeftOut)
	assert.Empty(t, out.Problems)
	assert.Empty(t, out.IgnoredColumns)
	require.Len(t, out.Findings, 1)
	f := out.Findings[0]
	assert.Equal(t, models.ShadowKindPlatformAgent, f.Kind)
	assert.Equal(t, "support-bot-prod", f.ExternalID)
	assert.Equal(t, "Customer support bot", f.DisplayName)
	assert.Equal(t, "production", f.Scope)
	assert.Equal(t, "active", f.State)
	assert.Equal(t, models.ShadowOwnerHint{Name: "Jane Perera", Email: "jane.perera@example.com"}, f.OwnerHint)
	require.NotNil(t, f.ProviderCreatedAt)
	assert.Equal(t, time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC), *f.ProviderCreatedAt)
	assert.Equal(t, map[string]any{
		"description": "Answers questions.", "source": "Internal platform",
		"agentUrl": "https://agents.example.com/support-bot", "framework": "LangGraph", "model": "gpt-4o",
		"agentVersion": "1.4.0", "labels": map[string]string{"team": "support", "region": "eu"},
	}, f.Attributes, "the url keeps only its scheme, host and path")
	require.Len(t, f.Signals, 1)
	assert.Equal(t, SigCustomAgent, f.Signals[0].Code)
	assert.Equal(t, "Uploaded CSV file", f.Signals[0].Source)
}

// Excel in some locales separates values with ";", and saves a UTF-8 BOM.
func TestInventoryCSVSemicolonsAndBOM(t *testing.T) {
	out := parseOK(t, "agents.csv", "\ufeffid;name;labels\r\nbot-1;Bot one;a=1, b=2\r\n")
	require.Len(t, out.Findings, 1)
	assert.Equal(t, "bot-1", out.Findings[0].ExternalID)
	assert.Equal(t, map[string]string{"a": "1, b=2"}, out.Findings[0].Attributes["labels"])

	// A header with commas keeps the comma, even when values have ";".
	out = parseOK(t, "agents.csv", "id,name,labels\nbot-1,Bot one,a=1;b=2\n")
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, out.Findings[0].Attributes["labels"])
}

// Rows are numbered as a spreadsheet shows them: the header is row 1, blank
// lines are rows, and a value spanning lines stays in one row.
func TestInventoryCSVRowNumbers(t *testing.T) {
	data := "id,name,labels\n" +
		"bot-1,Bot one,\"a=1\nb=2\"\n" + // row 2, two lines
		"\n" + // row 3, blank
		",No id,\n" + // row 4
		",,\n" + // row 5, empty: skipped
		"bot-1,Again,\n" // row 6
	out := parseOK(t, "a.csv", data)
	assert.Equal(t, 3, out.AgentsRead)
	assert.Equal(t, 2, out.LeftOut)
	require.Len(t, out.Problems, 2)
	assert.Equal(t, InventoryProblem{Row: 4, Message: "No id; the agent was left out.", LeftOut: true}, out.Problems[0])
	assert.Equal(t, 6, out.Problems[1].Row)
	assert.Contains(t, out.Problems[1].Message, `"bot-1" is listed in an earlier row`)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, out.Findings[0].Attributes["labels"],
		"labels separated by new lines")
}

func TestInventoryUnknownColumnsAreIgnored(t *testing.T) {
	out := parseOK(t, "a.csv", "id,name,Secret Token,cost,,Name\nbot-1,Bot,s3cr3t,10,,Other name\n")
	assert.Equal(t, []string{"Name", "Secret Token", "cost"}, out.IgnoredColumns,
		"unknown and repeated columns, sorted; an empty header is not listed")
	f := out.Findings[0]
	assert.Equal(t, "Bot", f.DisplayName, "the first name column is used")
	for _, v := range f.Attributes {
		assert.NotEqual(t, "s3cr3t", v)
	}
	assert.NotContains(t, fmt.Sprint(f), "s3cr3t", "an unknown column is never stored")
}

// A bad label, date or url is left out with a problem; the agent is kept.
func TestInventoryBadOptionalValuesKeepTheAgent(t *testing.T) {
	out := parseOK(t, "a.csv", "id,name,labels,createdAt,url\n"+
		"bot-1,Bot,\"ok=1; broken; =x\",15/01/2026,ftp://agents.example.com/x\n")
	require.Len(t, out.Findings, 1)
	f := out.Findings[0]
	assert.Zero(t, out.LeftOut)
	assert.Equal(t, map[string]string{"ok": "1"}, f.Attributes["labels"])
	assert.Nil(t, f.ProviderCreatedAt)
	assert.NotContains(t, f.Attributes, "agentUrl")
	require.Len(t, out.Problems, 3)
	assert.Equal(t, 3, out.ProblemCount)
	for _, p := range out.Problems {
		assert.Equal(t, 2, p.Row)
		assert.False(t, p.LeftOut, "the agent was kept")
	}
	assert.Contains(t, out.Problems[0].Message, "createdAt")
	assert.Contains(t, out.Problems[1].Message, "url")
	assert.Contains(t, out.Problems[2].Message, `"broken", "=x"`)
}

func TestInventoryDates(t *testing.T) {
	for in, want := range map[string]time.Time{
		"2026-01-15":                time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		"2026-01-15T09:30:00Z":      time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
		"2026-01-15T09:30:00+05:30": time.Date(2026, 1, 15, 4, 0, 0, 0, time.UTC),
		"2026-01-15T09:30:00.5Z":    time.Date(2026, 1, 15, 9, 30, 0, 500000000, time.UTC),
		"2026-01-15 09:30":          time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
		"2026-01-15 09:30:10":       time.Date(2026, 1, 15, 9, 30, 10, 0, time.UTC),
	} {
		got, ok := parseInventoryDate(in, false)
		require.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := parseInventoryDate("46037", false)
	assert.False(t, ok, "a number is a date only in Excel")
	got, ok := parseInventoryDate("46037.5", true)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), got)
	for _, bad := range []string{"yesterday", "15/01/2026", "2026-13-01", "0"} {
		_, ok := parseInventoryDate(bad, true)
		assert.False(t, ok, bad)
	}
}

func TestInventoryRequiredFieldsAndIDs(t *testing.T) {
	long := strings.Repeat("x", maxInventoryID+1)
	out := parseOK(t, "a.csv", "id,name\n"+
		"ok,Fine\n"+
		",No id\n"+
		"no-name,\n"+
		long+",Long id\n"+
		strings.Repeat("é", maxInventoryID)+",Long but fine\n")
	assert.Equal(t, 5, out.AgentsRead)
	assert.Equal(t, 3, out.LeftOut)
	assert.Len(t, out.Findings, 2, "an id of 256 characters is kept")
	assert.Contains(t, out.Problems[1].Message, "No name")
	assert.Contains(t, out.Problems[2].Message, "longer than 256")

	parseFails(t, "a.csv", []byte("name,owner\nBot,Jane\n"), "no id column")
	parseFails(t, "a.csv", []byte("ID\nbot\n"), "no name column")
	parseFails(t, "a.csv", []byte("id,name\n,x\ny,\n"), "None of the 2 agents in the file can be used")
	parseFails(t, "a.csv", []byte(""), "The file is empty")
	parseFails(t, "a.csv", []byte("\n\n"), "The file is empty")
	parseFails(t, "a.csv", []byte("id,name\n\xff\xfe,bad\n"), "not UTF-8")

	// A header alone is an empty inventory: every agent is gone.
	out = parseOK(t, "a.csv", "id,name\n")
	assert.Zero(t, out.AgentsRead)
	assert.Empty(t, out.Findings)
}

func TestInventoryProblemsAreCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("id,name\nkeep,Kept\n")
	for range 80 {
		b.WriteString("keep,Repeated\n")
	}
	out := parseOK(t, "a.csv", b.String())
	assert.Equal(t, 80, out.LeftOut)
	assert.Len(t, out.Problems, MaxInventoryProblems)
	assert.Equal(t, 80, out.ProblemCount, "every problem is counted, beyond the first 50")
	assert.Equal(t, 3, out.Problems[0].Row)
	assert.True(t, out.Problems[0].LeftOut)
}

// A row of many bad labels quotes the first few, and no problem message is
// longer than maxProblemMessage.
func TestInventoryProblemMessagesAreBounded(t *testing.T) {
	out := parseOK(t, "a.csv", "id,name,labels\nbot,Bot,\"a;b;c;d;e;f\"\n")
	require.Len(t, out.Problems, 1)
	assert.Equal(t, `labels "a", "b", "c" and 3 more are not key=value pairs; they were left out.`,
		out.Problems[0].Message)

	out = parseOK(t, "a.csv", "id,name,labels\nbot,Bot,\""+strings.Repeat("a;", 100000)+"\"\n")
	require.Len(t, out.Problems, 1)
	assert.Contains(t, out.Problems[0].Message, "and 99997 more")
	assert.LessOrEqual(t, len([]rune(out.Problems[0].Message)), maxProblemMessage)
}

// Warnings about a row's values are given only for an agent that is kept:
// a row left out has one problem, saying why.
func TestInventoryWarningsOnlyForKeptAgents(t *testing.T) {
	out := parseOK(t, "a.json", `{"agents":[
		{"framework": ["x"], "createdAt": "never", "name": "No id"},
		{"description": {"a": 1}, "id": ["x"], "name": "Bad id"},
		{"id": "dup", "name": "Kept", "model": ["x"]},
		{"description": ["x"], "labels": ["a"], "id": "dup", "name": "Repeated"}
	]}`)
	assert.Equal(t, 3, out.LeftOut)
	assert.Equal(t, 4, out.ProblemCount)
	require.Len(t, out.Problems, 4)
	for _, p := range out.Problems {
		assert.Equal(t, p.Row != 3, p.LeftOut, "row %d: %s", p.Row, p.Message)
	}
	assert.Contains(t, out.Problems[2].Message, "model must be text")

	out = parseOK(t, "a.csv", "id,name,createdAt,url,labels\nok,Kept,x,y,z\n,No id,x,y,z\n")
	assert.Equal(t, 4, out.ProblemCount)
	assert.Equal(t, 1, out.LeftOut)
	assert.True(t, out.Problems[3].LeftOut)
	assert.Equal(t, 3, out.Problems[3].Row)
}

// NUL and other control characters, which the database cannot store, are
// dropped from every value; tab and new line are kept.
func TestInventoryControlCharactersAreDropped(t *testing.T) {
	out := parseOK(t, "a.csv", "id,name,description,labels,bad\x00col\n"+
		"x\x00y,Bot\x01,\"line one\nline two\ttab\x7f\",\"k\x00=v\x1b\",z\n")
	f := findingByID(t, out, "xy")
	assert.Equal(t, "Bot", f.DisplayName)
	assert.Equal(t, "line one\nline two\ttab", f.Attributes["description"])
	assert.Equal(t, map[string]string{"k": "v"}, f.Attributes["labels"])
	assert.Equal(t, []string{"badcol"}, out.IgnoredColumns)

	out = parseOK(t, "a.json", `{"agents":[{"id":"x\u0000y","name":"Bot","description":"d\u0000",`+
		`"labels":{"k\u0000":"v\u0000"},"ext\u0000ra":1},{"id":"\u0000","name":"Only NUL"}]}`)
	f = findingByID(t, out, "xy")
	assert.Equal(t, "d", f.Attributes["description"])
	assert.Equal(t, map[string]string{"k": "v"}, f.Attributes["labels"])
	assert.Equal(t, []string{"extra"}, out.IgnoredColumns)
	require.Len(t, out.Problems, 1)
	assert.Equal(t, "No id; the agent was left out.", out.Problems[0].Message, "an id of only NUL is no id")
	assert.NotContains(t, fmt.Sprint(out.Findings), "\x00")
}

func TestInventoryLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("id,name\n")
	for i := range MaxInventoryAgents {
		fmt.Fprintf(&b, "a%d,Agent\n", i)
	}
	out := parseOK(t, "a.csv", b.String())
	assert.Len(t, out.Findings, MaxInventoryAgents)
	b.WriteString("one-more,Agent\n")
	parseFails(t, "a.csv", []byte(b.String()), "more than 5000 agents")

	items := make([]map[string]string, MaxInventoryAgents+1)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprint(i), "name": "A"}
	}
	js, err := json.Marshal(map[string]any{"agents": items})
	require.NoError(t, err)
	parseFails(t, "a.json", js, "more than 5000 agents")

	parseFails(t, "a.csv", bytes.Repeat([]byte("a"), MaxInventoryFileBytes+1), "larger than 5 MB")
}

func TestInventoryJSON(t *testing.T) {
	data := `{"agents": [
		{"id": "bot-1", "name": "Bot one", "Owner Email": "jane@example.com", "version": 2,
		 "labels": {"team": "support", "tier": 1}, "createdAt": "2026-01-15", "extra": {"x": 1}},
		{"id": "bot-2", "name": "Bot two", "labels": "a=1; b=2", "model": ["gpt"]},
		"not an object",
		{"id": 7, "name": "Numeric id"},
		{"id": ["x"], "name": "Bad id"},
		{"id": "bot-1", "name": "Repeated"},
		{"name": "No id"},
		{"id": "bot-3", "name": "Bad labels", "labels": ["a"]}
	], "version": 1}`
	out := parseOK(t, "a.json", data)
	assert.Equal(t, InventoryFormatJSON, out.Format)
	assert.Equal(t, 8, out.AgentsRead)
	assert.Equal(t, 4, out.LeftOut)
	assert.Equal(t, []string{"extra"}, out.IgnoredColumns)
	require.Len(t, out.Findings, 4)

	f := findingByID(t, out, "bot-1")
	assert.Equal(t, "jane@example.com", f.OwnerHint.Email)
	assert.Equal(t, "2", f.Attributes["agentVersion"])
	assert.Equal(t, map[string]string{"team": "support", "tier": "1"}, f.Attributes["labels"])
	assert.Equal(t, "Uploaded JSON file", f.Signals[0].Source)
	f = findingByID(t, out, "bot-2")
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, f.Attributes["labels"])
	assert.NotContains(t, f.Attributes, "model")
	findingByID(t, out, "7")
	assert.NotContains(t, findingByID(t, out, "bot-3").Attributes, "labels")

	rows := map[int]string{}
	for _, p := range out.Problems {
		rows[p.Row] += p.Message
	}
	assert.Contains(t, rows[2], "model must be text")
	assert.Contains(t, rows[3], "Not an object")
	assert.Contains(t, rows[5], "id must be text")
	assert.Contains(t, rows[6], "earlier row")
	assert.Contains(t, rows[7], "No id")
	assert.Contains(t, rows[8], "labels must be an object")

	// A bare list, with a BOM.
	out = parseOK(t, "a.json", "\ufeff[{\"id\":\"a\",\"name\":\"A\"}]")
	assert.Len(t, out.Findings, 1)

	parseFails(t, "a.json", []byte(`{"agents": [`), "not valid JSON")
	parseFails(t, "a.json", []byte(`{"agents": []} {}`), "more data")
	parseFails(t, "a.json", []byte(`{"items": []}`), `no "agents" list`)
	parseFails(t, "a.json", []byte(`{"agents": {"id": "a"}}`), `must be a list`)
	parseFails(t, "a.json", []byte(`{"agents": null}`), `must be a list`)
	parseFails(t, "a.json", []byte(`"agents"`), `must be an object`)
	parseFails(t, "a.json", []byte(`[{"name": "x"}]`), "None of the 1 agents")
}

// buildXLSX writes a workbook whose first sheet has rows; a time.Time is
// written as a date cell.
func buildXLSX(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_, err := f.NewSheet("Second")
	require.NoError(t, err)
	require.NoError(t, f.SetCellStr("Second", "A1", "not read"))
	for r, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, r+1)
		require.NoError(t, err)
		require.NoError(t, f.SetSheetRow("Sheet1", cell, &row))
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func TestInventoryXLSX(t *testing.T) {
	created := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	data := buildXLSX(t, [][]any{
		{"Id", "Name", "Created At", "Version", "Labels", "Notes"},
		{"bot-1", "Bot one", created, 1.5, "team=a\nregion=eu", "ignored"},
		{},
		{"", "No id"},
		{"bot-2", "Bot two", "2025-11-03"},
	})
	out, err := ParseInventoryFile("Inventory.XLSX", data)
	require.NoError(t, err)
	assert.Equal(t, InventoryFormatXLSX, out.Format)
	assert.Equal(t, 3, out.AgentsRead)
	assert.Equal(t, 1, out.LeftOut)
	assert.Equal(t, []string{"Notes"}, out.IgnoredColumns)
	require.Len(t, out.Problems, 1)
	assert.Equal(t, 4, out.Problems[0].Row, "the spreadsheet row, the empty row counted")
	f := findingByID(t, out, "bot-1")
	require.NotNil(t, f.ProviderCreatedAt, "a date cell is read as a date")
	assert.Equal(t, created, *f.ProviderCreatedAt)
	assert.Equal(t, "1.5", f.Attributes["agentVersion"])
	assert.Equal(t, map[string]string{"team": "a", "region": "eu"}, f.Attributes["labels"])
	assert.Equal(t, "Uploaded Excel file", f.Signals[0].Source)
	require.NotNil(t, findingByID(t, out, "bot-2").ProviderCreatedAt)

	parseFails(t, "a.xlsx", buildXLSX(t, [][]any{{"name"}, {"x"}}), "no id column")
	parseFails(t, "a.xlsx", buildXLSX(t, [][]any{{"Agents"}, {"id", "name"}}),
		"The first row of the file must hold the column names")
	parseFails(t, "a.xlsx", []byte("PK not really a zip"), "not a valid Excel workbook")
}

// rawXLSX writes a minimal workbook with the given shared strings and
// first sheet rows (sheetData XML), a second sheet, and extra parts given as
// name, content pairs.
func rawXLSX(t *testing.T, sst, rows, second string, extra ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	add := func(name, body string) {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	const ns = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types `+
		`xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/xl/workbook.xml" `+
		`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`+
		`<Override PartName="/xl/worksheets/sheet1.xml" `+
		`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`+
		`<Override PartName="/xl/worksheets/sheet2.xml" `+
		`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships `+
		`xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" `+
		`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" `+
		`Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook `+ns+
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`+
		`<sheet name="Agents" sheetId="1" r:id="rId1"/><sheet name="Notes" sheetId="2" r:id="rId3"/>`+
		`</sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships `+
		`xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" `+
		`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" `+
		`Target="worksheets/sheet1.xml"/>`+
		`<Relationship Id="rId3" `+
		`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" `+
		`Target="worksheets/sheet2.xml"/>`+
		`<Relationship Id="rId2" `+
		`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" `+
		`Target="sharedStrings.xml"/></Relationships>`)
	add("xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8"?><sst `+ns+`>`+sst+`</sst>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8"?><worksheet `+ns+`><sheetData>`+
		rows+`</sheetData></worksheet>`)
	add("xl/worksheets/sheet2.xml", `<?xml version="1.0" encoding="UTF-8"?><worksheet `+ns+`><sheetData>`+
		second+`</sheetData></worksheet>`)
	for i := 0; i+1 < len(extra); i += 2 {
		add(extra[i], extra[i+1])
	}
	require.NoError(t, z.Close())
	return b.Bytes()
}

// An Excel file that unpacks to too much, or whose first sheet is too wide
// to read row by row, is refused before excelize reads it.
func TestInventoryXLSXLimits(t *testing.T) {
	header := `<row r="1"><c r="A1" t="inlineStr"><is><t>id</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>name</t></is></c></row>`
	agent := `<row r="2"><c r="A2" t="inlineStr"><is><t>bot</t></is></c>` +
		`<c r="B2" t="inlineStr"><is><t>Bot</t></is></c></row>`
	wide := `<row r="3"><c r="XFD3"><v>1</v></c></row>`

	out, err := ParseInventoryFile("a.xlsx", rawXLSX(t, "", header+agent, header+wide))
	require.NoError(t, err, "only the first sheet's width counts")
	assert.Len(t, out.Findings, 1)

	parseFails(t, "a.xlsx", rawXLSX(t, "", header+agent+wide, ""), "column XFD (row 3)")
	// Empty but formatted cells far right are not read as values.
	_, err = ParseInventoryFile("a.xlsx", rawXLSX(t, "", header+agent+`<row r="3"><c r="XFD3" s="1"/></row>`, ""))
	require.NoError(t, err)
	// Cells without a reference are counted along the row.
	parseFails(t, "a.xlsx", rawXLSX(t, "", header+agent+`<row r="3">`+
		strings.Repeat(`<c><v>1</v></c>`, maxXLSXColumns+1)+`</row>`, ""), "column GS")

	// An empty value far right is not read either.
	_, err = ParseInventoryFile("a.xlsx", rawXLSX(t, "", header+agent+`<row r="3"><c r="XFD3" t="str"><v></v></c></row>`, ""))
	require.NoError(t, err)

	bigSST := strings.Repeat(`<si><t>a</t></si>`, maxXLSXPartBytes/17+1)
	parseFails(t, "a.xlsx", rawXLSX(t, bigSST, header+agent, ""), "unpacks to too much data")
	text := func(n int) string { return `<si><t>` + strings.Repeat("a", n) + `</t></si>` }
	part := text(maxXLSXPartBytes - 300)
	parseFails(t, "a.xlsx", rawXLSX(t, part, header+agent+`<row r="3">`+part+`</row>`,
		`<row r="1">`+part+`</row>`), "unpacks to too much data")

	// Shared strings that excelize would decode into far more memory: too
	// many strings, or too many elements in them.
	parseFails(t, "a.xlsx", rawXLSX(t, strings.Repeat(`<si/>`, maxXLSXStrings+1), header+agent, ""),
		"too many different text values")
	parseFails(t, "a.xlsx", rawXLSX(t, `<si>`+strings.Repeat(`<r><t/></r>`, maxXLSXStringTags/3+1)+`</si>`,
		header+agent, ""), "too many different text values")

	// Styles and other parts excelize decodes whole are small.
	styles := func(n int) string {
		return `<?xml version="1.0" encoding="UTF-8"?><styleSheet><cellXfs>` + strings.Repeat(`<xf/>`, n/5) +
			`</cellXfs></styleSheet>`
	}
	parseFails(t, "a.xlsx", rawXLSX(t, "", header+agent, "", "xl/styles.xml", styles(maxXLSXOtherXMLBytes)),
		"unpacks to too much data")
	parseFails(t, "a.xlsx", rawXLSX(t, "", header+agent, "", "xl/styles.xml", styles(800<<10),
		"xl/theme/theme1.xml", styles(800<<10), "xl/calcChain.xml", styles(800<<10)), "unpacks to too much data")
	// A picture is held, not decoded: only the total counts.
	_, err = ParseInventoryFile("a.xlsx", rawXLSX(t, "", header+agent, "", "xl/media/image1.png",
		strings.Repeat("x", 3<<20)))
	require.NoError(t, err)
}

// A full-size workbook as Excel writes it, with shared strings and a few
// hundred styles, is read.
func TestInventoryXLSXFullSize(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	header := make([]any, 0, len(inventoryFields))
	for _, fl := range inventoryFields {
		header = append(header, fl.Name)
	}
	require.NoError(t, f.SetSheetRow("Sheet1", "A1", &header))
	styles := make([]int, 0, 300)
	for i := range 300 {
		font := &excelize.Font{Size: float64(8 + i%20), Bold: i%2 == 0}
		id, err := f.NewStyle(&excelize.Style{Font: font, NumFmt: i % 50})
		require.NoError(t, err)
		styles = append(styles, id)
	}
	for r := 2; r <= MaxInventoryAgents+1; r++ {
		row := make([]any, 0, len(inventoryFields))
		for c := range inventoryFields {
			row = append(row, fmt.Sprintf("value %d of row %d, column %d", c, r, c))
		}
		row[0], row[4] = fmt.Sprintf("agent-%05d", r), fmt.Sprintf("https://agents.example.com/%d", r)
		row[12], row[13] = "2026-01-15", "team=t; region=eu"
		cell, err := excelize.CoordinatesToCellName(1, r)
		require.NoError(t, err)
		require.NoError(t, f.SetSheetRow("Sheet1", cell, &row))
		require.NoError(t, f.SetCellStyle("Sheet1", cell, cell, styles[r%len(styles)]))
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	out, err := ParseInventoryFile("agents.xlsx", buf.Bytes())
	require.NoError(t, err)
	assert.Len(t, out.Findings, MaxInventoryAgents)
	assert.Zero(t, out.ProblemCount)
}

// Format from the extension, else from the content; legacy .xls refused.
func TestInventoryFormatDetection(t *testing.T) {
	xlsx := buildXLSX(t, [][]any{{"id", "name"}, {"a", "A"}})
	for _, tc := range []struct{ name, data, format string }{
		{"agents.json", `{"agents": []}`, InventoryFormatJSON},
		{"agents.Json", `[]`, InventoryFormatJSON},
		{"agents.csv", "id,name\n", InventoryFormatCSV},
		{"agents.xlsx", string(xlsx), InventoryFormatXLSX},
		{"", string(xlsx), InventoryFormatXLSX},
		{"agents.txt", "  \n[{\"id\":\"a\",\"name\":\"A\"}]", InventoryFormatJSON},
		{"agents", `{"agents": []}`, InventoryFormatJSON},
	} {
		out, err := ParseInventoryFile(tc.name, []byte(tc.data))
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.format, out.Format, tc.name)
	}
	ole := append([]byte{}, oleHeader...)
	ole = append(ole, make([]byte, 100)...)
	parseFails(t, "old.xls", ole, ".xls")
	parseFails(t, "old.XLS", []byte("anything"), ".xls")
	parseFails(t, "noext", ole, ".xls")
	parseFails(t, "locked.xlsx", ole, "password-protected")
	parseFails(t, "agents.txt", []byte("id,name\na,A\n"), "Upload a .json, .csv or .xlsx file")
}

func TestInventoryFieldsAndCatalog(t *testing.T) {
	names := []string{}
	for _, f := range InventoryFields() {
		names = append(names, f.Name)
		assert.NotEmpty(t, f.Description, f.Name)
		assert.NotEmpty(t, f.Example, f.Name)
		assert.Equal(t, f.Name == InventoryFieldID || f.Name == InventoryFieldName, f.Required, f.Name)
	}
	assert.Equal(t, []string{
		"id", "name", "description", "source", "url", "framework", "model", "version", "status", "scope",
		"ownerName", "ownerEmail", "createdAt", "labels",
	}, names)
	assert.Len(t, inventorySecondExample, len(names))

	p, ok := Provider(models.ShadowProviderCustom)
	require.True(t, ok)
	assert.Equal(t, "Bring your own inventory", p.DisplayName)
	assert.True(t, p.FileUpload)
	assert.False(t, p.CloudScanned())
	assert.False(t, p.HoldsCredential())
	assert.Equal(t, InventoryFields(), p.InventoryFields)
}

// Each template reads back as two agents with every field set, in the
// format's column order.
func TestInventoryTemplates(t *testing.T) {
	for _, format := range []string{InventoryFormatJSON, InventoryFormatCSV, InventoryFormatXLSX} {
		body, err := InventoryTemplate(format)
		require.NoError(t, err, format)
		assert.Equal(t, "agent-inventory-template."+format, InventoryTemplateFileName(format))
		out, err := ParseInventoryFile(InventoryTemplateFileName(format), body)
		require.NoError(t, err, format)
		assert.Equal(t, 2, out.AgentsRead, format)
		assert.Empty(t, out.Problems, format)
		assert.Empty(t, out.IgnoredColumns, format)
		require.Len(t, out.Findings, 2, format)
		f := out.Findings[0]
		assert.Equal(t, "support-bot-prod", f.ExternalID, format)
		assert.Equal(t, "Customer support bot", f.DisplayName, format)
		assert.Equal(t, "Jane Perera", f.OwnerHint.Name, format)
		require.NotNil(t, f.ProviderCreatedAt, format)
		assert.Equal(t, map[string]string{"team": "support", "region": "eu"}, f.Attributes["labels"], format)
		assert.Len(t, f.Attributes, 7, format)
	}

	body, err := InventoryTemplate(InventoryFormatJSON)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), "{\n  \"agents\": [\n    {\n      \"id\": \"support-bot-prod\","),
		"pretty-printed, keys in format order")
	assert.Contains(t, string(body), `"labels": {`)

	body, err = InventoryTemplate(InventoryFormatCSV)
	require.NoError(t, err)
	header, err := csv.NewReader(bytes.NewReader(body)).Read()
	require.NoError(t, err)
	assert.Equal(t, "id", header[0])
	assert.Equal(t, "labels", header[len(header)-1])

	_, err = InventoryTemplate("xls")
	require.Error(t, err)
	assert.True(t, IsInventoryFormat("xlsx"))
	assert.False(t, IsInventoryFormat("XLSX"))
}
