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

// A small reader and writer of Excel workbooks (.xlsx, Office Open XML),
// built on archive/zip and encoding/xml only. The reader streams the first
// worksheet one row at a time and returns each cell's value as text; the
// writer writes a one-sheet workbook of text cells.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	// errXLSXInvalid is a file that is not a workbook: no workbook part, or
	// a workbook or shared strings part that is not valid XML.
	errXLSXInvalid = errors.New("not a valid Excel workbook")
	// errXLSXNoSheet is a workbook without a sheet.
	errXLSXNoSheet = errors.New("the workbook has no sheet")
	// errXLSXSheet is a first sheet that cannot be found or read.
	errXLSXSheet = errors.New("the first sheet cannot be read")
	// errXLSXTooManyStrings is shared strings beyond maxXLSXStrings.
	errXLSXTooManyStrings = errors.New("too many shared strings")
)

// xlsxWideCellError is a value (or formula) in a column beyond the widest
// read.
type xlsxWideCellError struct {
	Col, Row int
}

func (e *xlsxWideCellError) Error() string {
	return fmt.Sprintf("a value in column %s (row %d)", xlsxColumnName(e.Col), e.Row)
}

// xlsxWorkbook is an opened workbook: where its first sheet and shared
// strings are, and its date system.
type xlsxWorkbook struct {
	parts map[string]*zip.File
	// sheet is the first worksheet's part, in workbook order.
	sheet *zip.File
	// sharedStrings is the shared strings part, or nil.
	sharedStrings *zip.File
	// date1904 is the 1904 date system (dates count days from 1904-01-01).
	date1904 bool
}

// xlsxRelationship is one relationship of a relationships part.
type xlsxRelationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// openXLSX finds the first sheet of a workbook: the workbook part from the
// package relationships, its first <sheet>, and that sheet's relationship.
func openXLSX(data []byte) (*xlsxWorkbook, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errXLSXInvalid
	}
	wb := &xlsxWorkbook{parts: make(map[string]*zip.File, len(zr.File))}
	for _, zf := range zr.File {
		// Part names are compared ignoring case.
		name := xlsxPartKey(zf.Name)
		if _, dup := wb.parts[name]; !dup {
			wb.parts[name] = zf
		}
	}
	wbPath := "xl/workbook.xml"
	if rels, err := wb.readRels("_rels/.rels"); err == nil {
		for _, r := range rels {
			if strings.HasSuffix(r.Type, "/officeDocument") && r.TargetMode != "External" {
				wbPath = xlsxResolve("", r.Target)
				break
			}
		}
	}
	firstID, err := wb.readWorkbook(wbPath)
	if err != nil {
		return nil, err
	}
	rels, err := wb.readRels(path.Join(path.Dir(wbPath), "_rels", path.Base(wbPath)+".rels"))
	if err != nil {
		return nil, errXLSXSheet
	}
	dir := path.Dir(wbPath)
	for _, r := range rels {
		if r.TargetMode == "External" {
			continue
		}
		switch {
		case r.ID == firstID:
			if !strings.HasSuffix(r.Type, "/worksheet") {
				return nil, errXLSXSheet
			}
			wb.sheet = wb.parts[xlsxPartKey(xlsxResolve(dir, r.Target))]
		case strings.HasSuffix(r.Type, "/sharedStrings") && wb.sharedStrings == nil:
			wb.sharedStrings = wb.parts[xlsxPartKey(xlsxResolve(dir, r.Target))]
		}
	}
	if wb.sheet == nil {
		return nil, errXLSXSheet
	}
	if wb.sharedStrings == nil {
		wb.sharedStrings = wb.parts["xl/sharedstrings.xml"]
	}
	return wb, nil
}

// xlsxPartKey is a part name as looked up: without a leading "/", with
// "/" separators, in lower case.
func xlsxPartKey(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/"))
}

// xlsxResolve resolves a relationship target against the directory of the
// part the relationships belong to; a target starting with "/" is from the
// package root.
func xlsxResolve(dir, target string) string {
	target = strings.ReplaceAll(target, "\\", "/")
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean(path.Join("/", dir, target)), "/")
}

// maxXLSXDepth is the deepest nesting of elements read. The decoder holds
// every open element, so a few megabytes of nested elements would take
// hundreds; a sheet nests about ten deep.
const maxXLSXDepth = 64

var errXLSXTooDeep = errors.New("elements nested too deep")

// xlsxTokens reads a part's XML tokens, refusing elements nested too deep.
type xlsxTokens struct {
	d     *xml.Decoder
	depth int
}

func (x *xlsxTokens) next() (xml.Token, error) {
	tok, err := x.d.Token()
	switch tok.(type) {
	case xml.StartElement:
		x.depth++
		if x.depth > maxXLSXDepth {
			return nil, errXLSXTooDeep
		}
	case xml.EndElement:
		x.depth--
	}
	return tok, err
}

// decoder opens a part for an XML decoder.
func (wb *xlsxWorkbook) decoder(zf *zip.File, limit int64) (*xlsxTokens, io.Closer, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, nil, err
	}
	// checkXLSXSize bounds the declared sizes, and the zip reader fails on a
	// part longer than declared; the limit is a second guard.
	return &xlsxTokens{d: xml.NewDecoder(io.LimitReader(rc, limit))}, rc, nil
}

func (wb *xlsxWorkbook) readRels(name string) ([]xlsxRelationship, error) {
	zf, ok := wb.parts[xlsxPartKey(name)]
	if !ok {
		return nil, errXLSXInvalid
	}
	d, c, err := wb.decoder(zf, maxXLSXOtherXMLBytes+1)
	if err != nil {
		return nil, errXLSXInvalid
	}
	defer func() { _ = c.Close() }()
	var rels struct {
		Rels []xlsxRelationship `xml:"Relationship"`
	}
	// Decode limits the nesting itself.
	if err := d.d.Decode(&rels); err != nil {
		return nil, errXLSXInvalid
	}
	return rels.Rels, nil
}

// readWorkbook reads the workbook part's date system and returns the
// relationship id of its first sheet.
func (wb *xlsxWorkbook) readWorkbook(name string) (string, error) {
	zf, ok := wb.parts[xlsxPartKey(name)]
	if !ok {
		return "", errXLSXInvalid
	}
	d, c, err := wb.decoder(zf, maxXLSXOtherXMLBytes+1)
	if err != nil {
		return "", errXLSXInvalid
	}
	defer func() { _ = c.Close() }()
	firstID, sawSheet := "", false
	for {
		tok, err := d.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errXLSXInvalid
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "workbookPr":
			v := xmlAttr(start, "date1904")
			wb.date1904 = v == "1" || v == "true"
		case "sheet":
			if !sawSheet {
				sawSheet = true
				firstID = xmlAttr(start, "id")
			}
		}
	}
	if !sawSheet {
		return "", errXLSXNoSheet
	}
	return firstID, nil
}

// readSharedStrings reads the shared strings: each <si> is its <t> text, or
// the <t> texts of its rich text runs; phonetic runs (<rPh>) are not part
// of the text.
func (wb *xlsxWorkbook) readSharedStrings() ([]string, error) {
	if wb.sharedStrings == nil {
		return nil, nil
	}
	d, c, err := wb.decoder(wb.sharedStrings, maxXLSXPartBytes+1)
	if err != nil {
		return nil, errXLSXInvalid
	}
	defer func() { _ = c.Close() }()
	var out []string
	var text strings.Builder
	inSI, inT, phonetic := false, false, 0
	for {
		tok, err := d.next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, errXLSXInvalid
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				if len(out) >= maxXLSXStrings {
					return nil, errXLSXTooManyStrings
				}
				inSI = true
				text.Reset()
			case "rPh":
				phonetic++
			case "t":
				inT = inSI && phonetic == 0
			}
		case xml.CharData:
			if inT {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				if inSI {
					out = append(out, xlsxUnescape(text.String()))
				}
				inSI, inT, phonetic = false, false, 0
			case "rPh":
				phonetic = max(phonetic-1, 0)
			case "t":
				inT = false
			}
		}
	}
}

// xlsxCell is the cell being read.
type xlsxCell struct {
	col     int
	typ     string
	value   strings.Builder // <v>
	inline  strings.Builder // <is> text
	formula bool
	// nonEmpty is any <v> or <t> text, for a cell beyond the widest read.
	nonEmpty bool
}

// xlsxSheetState is where the sheet reader is.
type xlsxSheetState struct {
	inData, inRow, inCell bool
	inV, inIs, inT        bool
	phonetic              int
	row, col              int
	cell                  xlsxCell
}

// eachRow streams the first sheet and calls fn with each row's number (as
// a spreadsheet shows it, from 1) and its cell values, a cell's place being
// its column. Only the first maxCols columns are held: a value beyond them
// is an *xlsxWideCellError. Trailing empty cells are left out, and so are
// rows that are not in the sheet. cells is reused between calls.
func (wb *xlsxWorkbook) eachRow(sst []string, maxCols int, fn func(row int, cells []string) error) error {
	d, c, err := wb.decoder(wb.sheet, maxXLSXPartBytes+1)
	if err != nil {
		return errXLSXSheet
	}
	defer func() { _ = c.Close() }()
	var cells []string
	var s xlsxSheetState
	for {
		tok, err := d.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errXLSXSheet
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !s.inData {
				s.inData = t.Name.Local == "sheetData"
				continue
			}
			s.start(t)
			if t.Name.Local == "row" {
				cells = cells[:0]
			}
		case xml.CharData:
			if !s.inCell || (!s.inV && !s.inT) || len(t) == 0 {
				continue
			}
			s.cell.nonEmpty = true
			switch {
			case s.cell.col > maxCols:
			case s.inV:
				s.cell.value.Write(t)
			default:
				s.cell.inline.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "sheetData":
				s.inData = false
			case "row":
				if s.inRow {
					s.inRow = false
					if err := fn(s.row, cells); err != nil {
						return err
					}
				}
			case "c":
				if !s.inCell {
					continue
				}
				s.inCell, s.inV, s.inIs, s.inT, s.phonetic = false, false, false, false, 0
				if s.cell.col > maxCols {
					if s.cell.nonEmpty || s.cell.formula {
						return &xlsxWideCellError{Col: s.cell.col, Row: s.row}
					}
					continue
				}
				v, err := s.cell.text(sst)
				if err != nil {
					return err
				}
				if v != "" {
					for len(cells) < s.cell.col {
						cells = append(cells, "")
					}
					cells[s.cell.col-1] = v
				}
			case "v":
				s.inV = false
			case "is":
				s.inIs = false
			case "rPh":
				s.phonetic = max(s.phonetic-1, 0)
			case "t":
				s.inT = false
			}
		}
	}
}

// start handles an element that starts inside <sheetData>. A row or cell
// without a reference follows the one before it.
func (s *xlsxSheetState) start(t xml.StartElement) {
	switch t.Name.Local {
	case "row":
		s.row++
		if n, err := strconv.Atoi(xmlAttr(t, "r")); err == nil && n > 0 {
			s.row = n
		}
		s.inRow, s.col = true, 0
	case "c":
		if !s.inRow {
			return
		}
		s.col++
		if col, ok := xlsxCellColumn(xmlAttr(t, "r")); ok {
			s.col = col
		}
		s.inCell = true
		s.cell.col, s.cell.typ, s.cell.formula, s.cell.nonEmpty = s.col, xmlAttr(t, "t"), false, false
		s.cell.value.Reset()
		s.cell.inline.Reset()
	case "v":
		s.inV = s.inCell
	case "is":
		s.inIs = s.inCell
	case "rPh":
		s.phonetic++
	case "t":
		s.inT = s.inIs && s.phonetic == 0
	case "f":
		s.cell.formula = s.cell.formula || s.inCell
	}
}

// text is a cell's value as text: a number as written in the file (an Excel
// date is a number of days), a boolean as TRUE or FALSE, an error as its
// text (#N/A), and a formula as its last computed value.
func (c *xlsxCell) text(sst []string) (string, error) {
	v := c.value.String()
	switch c.typ {
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || i < 0 || i >= len(sst) {
			if strings.TrimSpace(v) == "" {
				return "", nil
			}
			return "", errXLSXSheet
		}
		return sst[i], nil
	case "inlineStr":
		if c.inline.Len() == 0 {
			return xlsxUnescape(v), nil
		}
		return xlsxUnescape(c.inline.String()), nil
	case "str":
		return xlsxUnescape(v), nil
	case "b":
		switch strings.TrimSpace(v) {
		case "1":
			return "TRUE", nil
		case "0":
			return "FALSE", nil
		}
		return v, nil
	default:
		// A number (no type, or "n"), an error ("e") or an ISO 8601 date
		// ("d"), as written.
		return v, nil
	}
}

// xlsxCellColumn reads the column of a cell reference such as "B7": A is 1,
// XFD 16384.
func xlsxCellColumn(ref string) (int, bool) {
	col, i := 0, 0
	for ; i < len(ref) && i < 4; i++ {
		ch := ref[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
			col = col*26 + int(ch-'A'+1)
		case ch >= 'a' && ch <= 'z':
			col = col*26 + int(ch-'a'+1)
		default:
			return col, col > 0 && col <= xlsxMaxColumns && xlsxIsRowNumber(ref[i:])
		}
	}
	return 0, false
}

// xlsxMaxColumns is the columns of a sheet, A to XFD.
const xlsxMaxColumns = 16384

func xlsxIsRowNumber(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// xlsxColumnName names a column: 1 is A, 27 AA.
func xlsxColumnName(col int) string {
	var b []byte
	for col > 0 {
		col--
		b = append([]byte{byte('A' + col%26)}, b...)
		col /= 26
	}
	return string(b)
}

// xlsxUnescape decodes the _xHHHH_ escapes Excel writes for characters XML
// cannot hold, such as _x000D_ for a carriage return; _x005F_ is "_".
func xlsxUnescape(s string) string {
	if !strings.Contains(s, "_x") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if i+7 <= len(s) && s[i] == '_' && s[i+1] == 'x' && s[i+6] == '_' {
			if n, err := strconv.ParseUint(s[i+2:i+6], 16, 16); err == nil {
				b.WriteRune(rune(n))
				i += 7
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// excelSerialTime reads an Excel date number: days since the start of the
// workbook's date system, the fraction being the time of day. In the 1900
// system day 1 is 1900-01-01 and day 60 is 1900-02-29, a day that never was
// (Excel keeps Lotus 1-2-3's leap year bug); in the 1904 system day 0 is
// 1904-01-01. The time is kept to the millisecond.
func excelSerialTime(n float64, date1904 bool) (time.Time, bool) {
	if math.IsNaN(n) || n < 1 || n >= 2958466 {
		return time.Time{}, false
	}
	days := math.Floor(n)
	var base time.Time
	switch {
	case date1904:
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	case days < 60:
		base = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	case days == 60:
		return time.Time{}, false
	default:
		base = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	}
	ms := math.Round((n - days) * 24 * 60 * 60 * 1000)
	t := base.AddDate(0, 0, int(days)).Add(time.Duration(ms) * time.Millisecond)
	if t.Year() > 9999 {
		return time.Time{}, false
	}
	return t, true
}

// ---- writer ----

// writeXLSX writes a workbook of one sheet named sheetName: every cell
// text, the first row bold, the columns colWidth characters wide.
func writeXLSX(sheetName string, rows [][]string, colWidth int) ([]byte, error) {
	const (
		xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
		nsMain    = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
		nsRel     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
		nsPkgRel  = "http://schemas.openxmlformats.org/package/2006/relationships"
		ctPrefix  = "application/vnd.openxmlformats-officedocument.spreadsheetml."
	)
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	var sheet bytes.Buffer
	sheet.WriteString(xmlHeader + `<worksheet xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">`)
	if width > 0 && len(rows) > 0 {
		fmt.Fprintf(&sheet, `<dimension ref="A1:%s%d"/>`, xlsxColumnName(width), len(rows))
		fmt.Fprintf(&sheet, `<cols><col min="1" max="%d" width="%d" customWidth="1"/></cols>`, width, colWidth)
	}
	sheet.WriteString(`<sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, r+1)
		for c, v := range row {
			style := ""
			if r == 0 {
				style = ` s="1"`
			}
			fmt.Fprintf(&sheet, `<c r="%s%d"%s t="inlineStr"><is><t`, xlsxColumnName(c+1), r+1, style)
			if strings.TrimSpace(v) != v {
				sheet.WriteString(` xml:space="preserve"`)
			}
			sheet.WriteByte('>')
			if err := xml.EscapeText(&sheet, []byte(v)); err != nil {
				return nil, fmt.Errorf("writing a cell: %w", err)
			}
			sheet.WriteString(`</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var name bytes.Buffer
	if err := xml.EscapeText(&name, []byte(sheetName)); err != nil {
		return nil, fmt.Errorf("writing the sheet name: %w", err)
	}
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xmlHeader +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="` + ctPrefix + `sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="` + ctPrefix + `worksheet+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="` + ctPrefix + `styles+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlHeader + `<Relationships xmlns="` + nsPkgRel + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", xmlHeader + `<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">` +
			`<bookViews><workbookView/></bookViews>` +
			`<sheets><sheet name="` + name.String() + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xmlHeader + `<Relationships xmlns="` + nsPkgRel + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + nsRel + `/styles" Target="styles.xml"/>` +
			`</Relationships>`},
		{"xl/styles.xml", xmlHeader + `<styleSheet xmlns="` + nsMain + `">` +
			`<fonts count="2">` +
			`<font><sz val="11"/><name val="Calibri"/><family val="2"/></font>` +
			`<font><b/><sz val="11"/><name val="Calibri"/><family val="2"/></font>` +
			`</fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill>` +
			`<fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
			`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
			`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
			`</styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	modified := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, p := range parts {
		w, err := z.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return nil, fmt.Errorf("writing the workbook: %w", err)
		}
		if _, err := io.WriteString(w, p.body); err != nil {
			return nil, fmt.Errorf("writing the workbook: %w", err)
		}
	}
	if err := z.Close(); err != nil {
		return nil, fmt.Errorf("writing the workbook: %w", err)
	}
	return out.Bytes(), nil
}
