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

package tableprinter

import (
	"fmt"
	"io"
	"strings"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

type FieldOption func(*field)

type field struct {
	text      string
	colorFunc func(string) string
}

func WithColor(fn func(string) string) FieldOption {
	return func(f *field) { f.colorFunc = fn }
}

type TablePrinter struct {
	out     io.Writer
	isTTY   bool
	cs      *iostreams.ColorScheme
	headers []string
	rows    [][]field
	current []field
}

func New(ios *iostreams.IOStreams, headers ...string) *TablePrinter {
	return &TablePrinter{
		out:     ios.Out,
		isTTY:   ios.IsStdoutTTY(),
		cs:      ios.ColorScheme(),
		headers: headers,
	}
}

func (tp *TablePrinter) AddField(text string, opts ...FieldOption) {
	f := field{text: text}
	for _, o := range opts {
		o(&f)
	}
	tp.current = append(tp.current, f)
}

func (tp *TablePrinter) EndRow() {
	tp.rows = append(tp.rows, tp.current)
	tp.current = nil
}

func (tp *TablePrinter) Render() error {
	if !tp.isTTY {
		return tp.renderTSV()
	}
	return tp.renderTable()
}

func (tp *TablePrinter) renderTSV() error {
	for _, row := range tp.rows {
		vals := make([]string, len(row))
		for i, f := range row {
			vals[i] = f.text
		}
		if _, err := fmt.Fprintln(tp.out, strings.Join(vals, "\t")); err != nil {
			return err
		}
	}
	return nil
}

func (tp *TablePrinter) renderTable() error {
	numCols := len(tp.headers)
	if numCols == 0 && len(tp.rows) > 0 {
		numCols = len(tp.rows[0])
	}
	if numCols == 0 {
		return nil
	}

	widths := make([]int, numCols)
	for i, h := range tp.headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, row := range tp.rows {
		for i, f := range row {
			if i < numCols && len(f.text) > widths[i] {
				widths[i] = len(f.text)
			}
		}
	}

	if len(tp.headers) > 0 {
		for i, h := range tp.headers {
			if i > 0 {
				fmt.Fprint(tp.out, "  ")
			}
			hdr := strings.ToUpper(h)
			if i < numCols-1 {
				hdr = padRight(hdr, widths[i])
			}
			fmt.Fprint(tp.out, tp.cs.TableHeader(hdr))
		}
		fmt.Fprintln(tp.out)
	}

	for _, row := range tp.rows {
		for i, f := range row {
			if i > 0 {
				fmt.Fprint(tp.out, "  ")
			}
			text := f.text
			if f.colorFunc != nil {
				text = f.colorFunc(text)
			}
			if i < numCols-1 {
				padding := widths[i] - len(f.text)
				if padding > 0 {
					text += strings.Repeat(" ", padding)
				}
			}
			fmt.Fprint(tp.out, text)
		}
		fmt.Fprintln(tp.out)
	}
	return nil
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
