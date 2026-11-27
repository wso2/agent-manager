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
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		got, ok := parseInventoryDate(in, noExcelDates)
		require.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := parseInventoryDate("46037", noExcelDates)
	assert.False(t, ok, "a number is a date only in Excel")
	for _, bad := range []string{"yesterday", "15/01/2026", "2026-13-01", "0", "-5", "NaN", "2958466", "60"} {
		_, ok := parseInventoryDate(bad, excelDates1900)
		assert.False(t, ok, bad)
	}
}

// Excel stores a date as a number of days: from 1900-01-01 as day 1, with
// the 1900-02-29 that never was as day 60, or from 1904-01-01 as day 0.
func TestInventoryExcelDates(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		in    string
		dates excelDates
		want  time.Time
	}{
		{"1", excelDates1900, day(1900, 1, 1)},
		{"59", excelDates1900, day(1900, 2, 28)},
		{"61", excelDates1900, day(1900, 3, 1)},
		{"46037", excelDates1900, day(2026, 1, 15)},
		{"46037.5", excelDates1900, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)},
		// 09:30 as Excel writes it.
		{"46037.395833333336", excelDates1900, time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)},
		{"4.6037E4", excelDates1900, day(2026, 1, 15)},
		{"2958465", excelDates1900, day(9999, 12, 31)},
		{"1", excelDates1904, day(1904, 1, 2)},
		{"44575", excelDates1904, day(2026, 1, 15)},
		{"44575.75", excelDates1904, time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC)},
		{"2026-01-15", excelDates1904, day(2026, 1, 15)},
	} {
		got, ok := parseInventoryDate(tc.in, tc.dates)
		require.True(t, ok, tc.in)
		assert.Equal(t, tc.want, got, "%s in date system %d", tc.in, tc.dates)
	}
	_, ok := parseInventoryDate("2958465", excelDates1904)
	assert.False(t, ok, "after 9999-12-31")
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

// xlsxRowsXML writes rows as the <row> elements of a sheet: a string as an
// inline string, a bool as a boolean, a time.Time as Excel stores a date (a
// number of days, with a date style) and any other value as a number.
func xlsxRowsXML(rows [][]any) string {
	var b strings.Builder
	for r, row := range rows {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, v := range row {
			ref := xlsxColumnName(c+1) + strconv.Itoa(r+1)
			switch v := v.(type) {
			case string:
				b.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t>`)
				_ = xml.EscapeText(&b, []byte(v))
				b.WriteString(`</t></is></c>`)
			case bool:
				n := 0
				if v {
					n = 1
				}
				fmt.Fprintf(&b, `<c r="%s" t="b"><v>%d</v></c>`, ref, n)
			case time.Time:
				days := float64(v.Sub(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC))) / float64(24*time.Hour)
				fmt.Fprintf(&b, `<c r="%s" s="1"><v>%s</v></c>`, ref, strconv.FormatFloat(days, 'g', -1, 64))
			default:
				fmt.Fprintf(&b, `<c r="%s"><v>%v</v></c>`, ref, v)
			}
		}
		b.WriteString(`</row>`)
	}
	return b.String()
}

// buildXLSX writes a workbook whose first sheet has rows (see xlsxRowsXML),
// and a second sheet that is not read.
func buildXLSX(t *testing.T, rows [][]any) []byte {
	t.Helper()
	return rawXLSX(t, "", xlsxRowsXML(rows), `<row r="1"><c r="A1" t="inlineStr"><is><t>not read</t></is></c></row>`)
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

const (
	xlsxNS = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	xlsxRelNS   = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
	xlsxRelType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)

// xlsxTestParts are the parts of a minimal workbook with the given shared
// strings and first sheet rows (sheetData XML), and a second sheet.
func xlsxTestParts(sst, rows, second string) map[string]string {
	const decl = `<?xml version="1.0" encoding="UTF-8"?>`
	return map[string]string{
		"[Content_Types].xml": decl + `<Types ` +
			`xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ` +
			`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ` +
			`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet2.xml" ` +
			`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels": decl + `<Relationships ` + xlsxRelNS + `><Relationship Id="rId1" ` +
			`Type="` + xlsxRelType + `officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": decl + `<workbook ` + xlsxNS + `><sheets>` +
			`<sheet name="Agents" sheetId="1" r:id="rId1"/><sheet name="Notes" sheetId="2" r:id="rId3"/>` +
			`</sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": decl + `<Relationships ` + xlsxRelNS + `>` +
			`<Relationship Id="rId1" Type="` + xlsxRelType + `worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId3" Type="` + xlsxRelType + `worksheet" Target="worksheets/sheet2.xml"/>` +
			`<Relationship Id="rId2" Type="` + xlsxRelType + `sharedStrings" Target="sharedStrings.xml"/>` +
			`</Relationships>`,
		"xl/sharedStrings.xml":     decl + `<sst ` + xlsxNS + `>` + sst + `</sst>`,
		"xl/worksheets/sheet1.xml": decl + `<worksheet ` + xlsxNS + `><sheetData>` + rows + `</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": decl + `<worksheet ` + xlsxNS + `><sheetData>` + second + `</sheetData></worksheet>`,
	}
}

// zipXLSX zips the parts, [Content_Types].xml first.
func zipXLSX(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "[Content_Types].xml") != (names[j] == "[Content_Types].xml") {
			return names[i] == "[Content_Types].xml"
		}
		return names[i] < names[j]
	})
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range names {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		require.NoError(t, err)
		_, err = w.Write([]byte(parts[name]))
		require.NoError(t, err)
	}
	require.NoError(t, z.Close())
	return b.Bytes()
}

// rawXLSX writes a minimal workbook with the given shared strings and
// first sheet rows (sheetData XML), a second sheet, and extra parts given as
// name, content pairs.
func rawXLSX(t *testing.T, sst, rows, second string, extra ...string) []byte {
	t.Helper()
	parts := xlsxTestParts(sst, rows, second)
	for i := 0; i+1 < len(extra); i += 2 {
		parts[extra[i]] = extra[i+1]
	}
	return zipXLSX(t, parts)
}

// An Excel file that unpacks to too much, or whose first sheet has a value
// beyond the widest read, is refused.
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

	// Shared strings that would be read into far more memory: too many
	// strings, or too many elements in them.
	parseFails(t, "a.xlsx", rawXLSX(t, strings.Repeat(`<si/>`, maxXLSXStrings+1), header+agent, ""),
		"too many different text values")
	parseFails(t, "a.xlsx", rawXLSX(t, `<si>`+strings.Repeat(`<r><t/></r>`, maxXLSXStringTags/3+1)+`</si>`,
		header+agent, ""), "too many different text values")

	// Styles and other XML parts are small.
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
	var sst, rows strings.Builder
	index := map[string]int{}
	shared := func(s string) int {
		i, ok := index[s]
		if !ok {
			i = len(index)
			index[s] = i
			sst.WriteString(`<si><t>`)
			_ = xml.EscapeText(&sst, []byte(s))
			sst.WriteString(`</t></si>`)
		}
		return i
	}
	writeRow := func(r int, values []string) {
		fmt.Fprintf(&rows, `<row r="%d" spans="1:%d">`, r, len(values))
		for c, v := range values {
			fmt.Fprintf(&rows, `<c r="%s%d" s="%d" t="s"><v>%d</v></c>`, xlsxColumnName(c+1), r, r%300, shared(v))
		}
		rows.WriteString(`</row>`)
	}
	header := make([]string, 0, len(inventoryFields))
	for _, fl := range inventoryFields {
		header = append(header, fl.Name)
	}
	writeRow(1, header)
	for r := 2; r <= MaxInventoryAgents+1; r++ {
		row := make([]string, 0, len(inventoryFields))
		for c := range inventoryFields {
			row = append(row, fmt.Sprintf("value %d of row %d, column %d", c, r, c))
		}
		row[0], row[4] = fmt.Sprintf("agent-%05d", r), fmt.Sprintf("https://agents.example.com/%d", r)
		row[12], row[13] = "2026-01-15", "team=t; region=eu"
		writeRow(r, row)
	}
	var styles strings.Builder
	styles.WriteString(`<?xml version="1.0" encoding="UTF-8"?><styleSheet ` + xlsxNS + `><fonts count="300">`)
	for i := range 300 {
		fmt.Fprintf(&styles, `<font><sz val="%d"/><name val="Calibri"/></font>`, 8+i%20)
	}
	styles.WriteString(`</fonts><cellXfs count="300">`)
	for i := range 300 {
		fmt.Fprintf(&styles, `<xf numFmtId="%d" fontId="%d" applyFont="1"/>`, i%50, i)
	}
	styles.WriteString(`</cellXfs></styleSheet>`)
	data := rawXLSX(t, sst.String(), rows.String(), "", "xl/styles.xml", styles.String())
	require.Less(t, len(data), MaxInventoryFileBytes)
	out, err := ParseInventoryFile("agents.xlsx", data)
	require.NoError(t, err)
	assert.Len(t, out.Findings, MaxInventoryAgents)
	assert.Zero(t, out.ProblemCount)
	assert.Equal(t, "value 1 of row 2, column 1", out.Findings[0].DisplayName)
}

// Workbooks as Excel, LibreOffice, Google Sheets and other tools write them.
func TestInventoryXLSXShapes(t *testing.T) {
	inline := func(ref, s string) string {
		return `<c r="` + ref + `" t="inlineStr"><is><t>` + s + `</t></is></c>`
	}
	header := `<row r="1">` + inline("A1", "id") + inline("B1", "name") + `</row>`
	// Excel: shared strings with rich text and phonetic runs, a dimension and
	// spans over every column, styled empty cells far right, and formulas
	// outside the cells.
	excelSST := `<si><t>id</t></si><si><t>name</t><rPh sb="0" eb="4"><t>ネーム</t></rPh></si><si><t>a1</t></si>` +
		`<si><r><t>Bot</t></r><r><rPr><b/></rPr><t xml:space="preserve"> One</t></r></si>`
	excelSheet := func(rows string) string {
		return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet ` + xlsxNS +
			` xmlns:x14ac="http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac">` +
			`<dimension ref="A1:XFD1048576"/><sheetViews><sheetView workbookViewId="0"/></sheetViews>` +
			`<cols><col min="1" max="16384" width="9"/></cols><sheetData>` + rows + `</sheetData>` +
			`<extLst><ext uri="{CCE6A557-97BC-4b89-ADB6-D9C93CAAB3DF}"><x14:dataValidations ` +
			`xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main" ` +
			`xmlns:xm="http://schemas.microsoft.com/office/excel/2006/main"><x14:dataValidation type="list">` +
			`<x14:formula1><xm:f>Notes!$A$1:$A$5</xm:f></x14:formula1><xm:sqref>ZZ2:ZZ100</xm:sqref>` +
			`</x14:dataValidation></x14:dataValidations></ext></extLst></worksheet>`
	}
	excelRows := `<row r="1" spans="1:16384" x14ac:dyDescent="0.25"><c r="A1" s="1" t="s"><v>0</v></c>` +
		`<c r="B1" s="1" t="s"><v>1</v></c><c r="XFD1" s="1"/></row>` +
		`<row r="2" spans="1:2"><c r="A2" t="s"><v>2</v></c><c r="B2" t="s"><v>3</v></c><c r="ZZ2" s="3"/></row>`
	wide := `<row r="1"><c r="A1"><v>1</v></c>` + inline("ZZ1", "far") + `</row>`
	workbook := func(sheets string) string {
		return `<?xml version="1.0"?><workbook ` + xlsxNS + `><sheets>` + sheets + `</sheets></workbook>`
	}
	bom := "\ufeff"

	for _, tc := range []struct {
		name   string
		sst    string
		rows   string
		change func(parts map[string]string)
		want   []string // id/name of each agent
	}{
		{
			name: "excel", sst: excelSST, want: []string{"a1/Bot One"},
			change: func(p map[string]string) {
				p["xl/worksheets/sheet1.xml"] = excelSheet(excelRows)
				p["xl/worksheets/sheet2.xml"] = excelSheet(wide)
				p["xl/workbook.xml"] = workbook(`<sheet name="Agents &amp; R&amp;D ü" sheetId="1" r:id="rId1"/>` +
					`<sheet name="Notes" sheetId="2" r:id="rId3"/>`)
			},
		},
		{
			name: "absolute targets and a byte order mark", sst: excelSST, want: []string{"a1/Bot One"},
			change: func(p map[string]string) {
				p["xl/worksheets/sheet1.xml"] = bom + excelSheet(excelRows)
				p["_rels/.rels"] = strings.Replace(p["_rels/.rels"], `"xl/workbook.xml"`, `"/xl/workbook.xml"`, 1)
				p["xl/_rels/workbook.xml.rels"] = strings.NewReplacer(`"worksheets/`, `"/xl/worksheets/`,
					`"sharedStrings.xml"`, `"/xl/sharedStrings.xml"`).Replace(p["xl/_rels/workbook.xml.rels"])
			},
		},
		{
			// Sheets moved in Excel: the first sheet is stored as sheet2.xml.
			name: "reordered sheets", sst: excelSST, rows: wide, want: []string{"a1/Bot One"},
			change: func(p map[string]string) {
				p["xl/worksheets/sheet2.xml"] = excelSheet(excelRows)
				p["xl/workbook.xml"] = workbook(`<sheet name="Agents" sheetId="2" r:id="rId3"/>` +
					`<sheet name="Notes" sheetId="1" r:id="rId1"/>`)
			},
		},
		{
			// Inline strings, as Apache POI's streaming writer writes them; a
			// formula's text result; a long number stays as written.
			name: "inline strings, formulas and numbers",
			rows: header + `<row r="2">` + inline("A2", "p1") +
				`<c r="B2" t="inlineStr"><is><r><t>Rich</t></r><r><t xml:space="preserve"> inline</t></r></is></c></row>` +
				`<row r="3"><c r="A3" t="n"><v>12345678901234567890</v></c>` +
				`<c r="B3" t="str"><f>"x"&amp;"y"</f><v>xy</v></c></row>` +
				`<row r="4"><c r="A4"><v>123456789012</v></c><c r="B4" t="b"><v>1</v></c></row>` +
				`<row r="5"><c r="A5" t="e"><v>#N/A</v></c><c r="B5" t="b"><v>0</v></c></row>`,
			want: []string{"p1/Rich inline", "12345678901234567890/xy", "123456789012/TRUE", "#N/A/FALSE"},
		},
		{
			name: "rows and cells without a reference",
			rows: `<row><c t="inlineStr"><is><t>id</t></is></c><c t="inlineStr"><is><t>name</t></is></c></row>` +
				`<row><c t="inlineStr"><is><t>q1</t></is></c><c t="inlineStr"><is><t>NoRef</t></is></c></row>` +
				`<row r="5"><c r="B5" t="inlineStr"><is><t>No id</t></is></c><c t="inlineStr"><is><t>x</t></is></c>` +
				`</row><row>` + inline("A6", "q2") + `<c><v>7</v></c></row>`,
			want: []string{"q1/NoRef", "q2/7"},
		},
		{
			name: "escaped characters",
			sst:  `<si><t>id</t></si><si><t>name</t></si><si><t>Line_x000D_ one_x005F_x0041_</t></si>`,
			rows: `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
				`<row r="2">` + inline("A2", "e1") + `<c r="B2" t="s"><v>2</v></c></row>`,
			want: []string{"e1/Line one_x0041_"},
		},
		{
			// An empty value far right, as a formula cell left blank.
			name: "empty value beyond the widest column",
			rows: header + `<row r="2">` + inline("A2", "e1") + inline("B2", "E") +
				`<c r="HA2" t="str"><v></v></c><c r="XFD2" s="2"/></row>`,
			want: []string{"e1/E"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := xlsxTestParts(tc.sst, tc.rows, "")
			if tc.change != nil {
				tc.change(parts)
			}
			out, err := ParseInventoryFile("a.xlsx", zipXLSX(t, parts))
			require.NoError(t, err)
			got := []string{}
			for _, f := range out.Findings {
				got = append(got, f.ExternalID+"/"+f.DisplayName)
			}
			assert.Equal(t, tc.want, got)
			assert.Empty(t, out.IgnoredColumns)
		})
	}

	t.Run("left out row is numbered as in the sheet", func(t *testing.T) {
		out, err := ParseInventoryFile("a.xlsx", rawXLSX(t, "", header+`<row>`+inline("A2", "k")+inline("B2", "K")+
			`</row><row r="9">`+inline("B9", "No id")+`</row>`, ""))
		require.NoError(t, err)
		require.Len(t, out.Problems, 1)
		assert.Equal(t, 9, out.Problems[0].Row)
	})
}

// Dates in Excel files: date cells, in either date system, and dates
// written as text.
func TestInventoryXLSXDates(t *testing.T) {
	rows := `<row r="1"><c r="A1" t="inlineStr"><is><t>id</t></is></c><c r="B1" t="inlineStr"><is><t>name</t></is></c>` +
		`<c r="C1" t="inlineStr"><is><t>createdAt</t></is></c><c r="D1" t="inlineStr"><is><t>version</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>d1</t></is></c><c r="B2" t="inlineStr"><is><t>D</t></is></c>` +
		`<c r="C2" s="1"><v>%s</v></c><c r="D2" s="1"><v>%s</v></c></row>`
	for _, tc := range []struct {
		name, workbookPr, serial string
		want                     time.Time
	}{
		{"1900", "", "46037", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{
			"1900 with a time", `<workbookPr date1904="0"/>`, "46037.395833333336",
			time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
		},
		{"1904", `<workbookPr date1904="1"/>`, "44575", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{"1904 as true", `<workbookPr date1904="true"/>`, "44575.5", time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := xlsxTestParts("", fmt.Sprintf(rows, tc.serial, tc.serial), "")
			parts["xl/workbook.xml"] = strings.Replace(parts["xl/workbook.xml"], "<sheets>", tc.workbookPr+"<sheets>", 1)
			out, err := ParseInventoryFile("a.xlsx", zipXLSX(t, parts))
			require.NoError(t, err)
			f := findingByID(t, out, "d1")
			require.NotNil(t, f.ProviderCreatedAt)
			assert.Equal(t, tc.want, *f.ProviderCreatedAt)
			assert.Equal(t, tc.serial, f.Attributes["agentVersion"], "only createdAt is read as a date")
		})
	}
}

// Files that are zips but not workbooks, or whose first sheet cannot be
// read, are refused with a message.
func TestInventoryXLSXBroken(t *testing.T) {
	header := `<row r="1"><c r="A1" t="inlineStr"><is><t>id</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>name</t></is></c></row>`
	for _, tc := range []struct {
		name   string
		rows   string
		change func(parts map[string]string)
		want   string
	}{
		{"no workbook", header, func(p map[string]string) { delete(p, "xl/workbook.xml") }, "not a valid Excel workbook"},
		{
			"workbook not XML", header, func(p map[string]string) { p["xl/workbook.xml"] = "<workbook><sheets>" },
			"not a valid Excel workbook",
		},
		{"no sheet", header, func(p map[string]string) {
			p["xl/workbook.xml"] = `<workbook ` + xlsxNS + `><sheets/></workbook>`
		}, "has no sheet"},
		{
			"sheet part missing", header, func(p map[string]string) { delete(p, "xl/worksheets/sheet1.xml") },
			"first sheet of the Excel workbook cannot be read",
		},
		{
			"workbook relationships missing", header, func(p map[string]string) { delete(p, "xl/_rels/workbook.xml.rels") },
			"first sheet of the Excel workbook cannot be read",
		},
		{
			"sheet not XML", header + `<row r="2"><c r="A2"><v>1</c></row>`, nil,
			"first sheet of the Excel workbook cannot be read",
		},
		{
			"shared string out of range", header + `<row r="2"><c r="A2" t="s"><v>5</v></c></row>`, nil,
			"first sheet of the Excel workbook cannot be read",
		},
		{
			"elements nested too deep", header + `<row r="2"><c r="A2">` + strings.Repeat("<a>", 10000), nil,
			"first sheet of the Excel workbook cannot be read",
		},
		{"no rows", "", nil, "The file is empty"},
		{"a title row above the column names", `<row r="1"><c r="A1" t="inlineStr"><is><t>Our agents</t></is></c>` +
			`</row>` + strings.ReplaceAll(header, `1"`, `2"`), nil, "no id column"},
		{"column names not in row 1", strings.ReplaceAll(header, `1"`, `3"`), nil, "no id column"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := xlsxTestParts("", tc.rows, "")
			if tc.change != nil {
				tc.change(parts)
			}
			parseFails(t, "a.xlsx", zipXLSX(t, parts), tc.want)
		})
	}
	// The shared strings are counted as they are read too, wherever the
	// workbook keeps them.
	parts := xlsxTestParts("", header, "")
	parts["xl/worksheets/strings.xml"] = `<sst>` + strings.Repeat(`<si/>`, maxXLSXStrings+1) + `</sst>`
	parts["xl/_rels/workbook.xml.rels"] = strings.Replace(parts["xl/_rels/workbook.xml.rels"],
		`"sharedStrings.xml"`, `"worksheets/strings.xml"`, 1)
	parseFails(t, "a.xlsx", zipXLSX(t, parts), "too many different text values")
	parts["xl/worksheets/strings.xml"] = `<sst><si>` + strings.Repeat(`<r>`, 10000) + `</si></sst>`
	parseFails(t, "a.xlsx", zipXLSX(t, parts), "not a valid Excel workbook")
}

func TestXLSXColumnsAndEscapes(t *testing.T) {
	for ref, col := range map[string]int{"A1": 1, "z9": 26, "AA1": 27, "GR10": 200, "GS1": 201, "XFD1048576": 16384} {
		got, ok := xlsxCellColumn(ref)
		require.True(t, ok, ref)
		assert.Equal(t, col, got, ref)
		assert.Equal(t, strings.ToUpper(strings.TrimRight(ref, "0123456789")), xlsxColumnName(col))
	}
	for _, bad := range []string{"", "A", "1", "XFE1", "AAAA1", "A1B", "$A$1"} {
		_, ok := xlsxCellColumn(bad)
		assert.False(t, ok, bad)
	}
	assert.Equal(t, "a\rb_c_x00_", xlsxUnescape("a_x000D_b_x005F_c_x00_"))
	assert.Equal(t, "_x000D_", xlsxUnescape("_x005F_x000D_"))
}

// The writer's workbook reads back exactly, and has the parts and content
// types of a minimal workbook.
func TestWriteXLSX(t *testing.T) {
	rows := [][]string{
		{"id", "name", "notes"},
		{"a&b <c>", " leading and trailing ", "line one\nline two\ttab"},
		{"ü", "", "\"quoted\""},
	}
	data, err := writeXLSX("R&D ü", rows, 24)
	require.NoError(t, err)

	wb, err := openXLSX(data)
	require.NoError(t, err)
	assert.False(t, wb.date1904)
	assert.Nil(t, wb.sharedStrings)
	var got [][]string
	require.NoError(t, wb.eachRow(nil, maxXLSXColumns, func(row int, cells []string) error {
		assert.Equal(t, len(got)+1, row)
		got = append(got, append([]string{}, cells...))
		return nil
	}))
	want := [][]string{rows[0], rows[1], {"ü", "", "\"quoted\""}}
	assert.Equal(t, want, got)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	parts := map[string]string{}
	for i, zf := range zr.File {
		rc, err := zf.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		parts[zf.Name] = string(b)
		if i == 0 {
			assert.Equal(t, "[Content_Types].xml", zf.Name, "the content types come first")
		}
		// Every part is well-formed XML.
		d := xml.NewDecoder(bytes.NewReader(b))
		for {
			_, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err, zf.Name)
		}
	}
	assert.Len(t, parts, 6)
	ct := parts["[Content_Types].xml"]
	for _, s := range []string{
		`PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"`,
		`PartName="/xl/worksheets/sheet1.xml" ` +
			`ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"`,
		`PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"`,
	} {
		assert.Contains(t, ct, s)
	}
	assert.Contains(t, parts["xl/workbook.xml"], `<sheet name="R&amp;D ü" sheetId="1" r:id="rId1"/>`)
	assert.Contains(t, parts["xl/styles.xml"], `<font><b/>`)
	sheet := parts["xl/worksheets/sheet1.xml"]
	assert.Contains(t, sheet, `<c r="A1" s="1" t="inlineStr"><is><t>id</t></is></c>`, "the header is bold")
	assert.Contains(t, sheet, `<c r="A2" t="inlineStr">`, "other rows are not")
	assert.Contains(t, sheet, `<t xml:space="preserve"> leading and trailing </t>`)
	assert.Contains(t, sheet, `<dimension ref="A1:C3"/>`)
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

	body, err = InventoryTemplate(InventoryFormatXLSX)
	require.NoError(t, err)
	wb, err := openXLSX(body)
	require.NoError(t, err)
	var got [][]string
	require.NoError(t, wb.eachRow(nil, maxXLSXColumns, func(_ int, cells []string) error {
		got = append(got, append([]string{}, cells...))
		return nil
	}))
	assert.Equal(t, inventoryTemplateRows(), got, "the Excel template holds the rows as text")
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	for _, zf := range zr.File {
		if zf.Name != "xl/workbook.xml" && zf.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, err := zf.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		if zf.Name == "xl/workbook.xml" {
			assert.Contains(t, string(b), `<sheet name="Agents" sheetId="1" r:id="rId1"/>`)
		} else {
			assert.Contains(t, string(b), `<col min="1" max="14" width="24" customWidth="1"/>`)
			assert.Contains(t, string(b), `<c r="N1" s="1" t="inlineStr"><is><t>labels</t></is></c>`)
		}
	}

	_, err = InventoryTemplate("xls")
	require.Error(t, err)
	assert.True(t, IsInventoryFormat("xlsx"))
	assert.False(t, IsInventoryFormat("XLSX"))
}
