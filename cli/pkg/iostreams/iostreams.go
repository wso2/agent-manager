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

package iostreams

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	JSON   bool

	stdinIsTerminal  bool
	stdoutIsTerminal bool
	stderrIsTerminal bool
}

func System() *IOStreams {
	return &IOStreams{
		In:               os.Stdin,
		Out:              os.Stdout,
		ErrOut:           os.Stderr,
		stdinIsTerminal:  term.IsTerminal(int(os.Stdin.Fd())),
		stdoutIsTerminal: term.IsTerminal(int(os.Stdout.Fd())),
		stderrIsTerminal: term.IsTerminal(int(os.Stderr.Fd())),
	}
}

func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return &IOStreams{In: in, Out: out, ErrOut: errOut}, in, out, errOut
}

func (s *IOStreams) CanPrompt() bool {
	return s.stdinIsTerminal && s.stderrIsTerminal
}

func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutIsTerminal }

func (s *IOStreams) ColorScheme() *ColorScheme {
	return &ColorScheme{Enabled: s.stdoutIsTerminal}
}

func (s *IOStreams) StderrColorScheme() *ColorScheme {
	return &ColorScheme{Enabled: s.stderrIsTerminal}
}

func (s *IOStreams) SetTerminal(stdin, stdout, stderr bool) {
	s.stdinIsTerminal = stdin
	s.stdoutIsTerminal = stdout
	s.stderrIsTerminal = stderr
}
