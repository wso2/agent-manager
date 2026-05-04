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

package prompter

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Prompter interface {
	ConfirmDeletion(required string) error
}

type linePrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) Prompter {
	return &linePrompter{in: bufio.NewReader(in), out: out}
}

func (p *linePrompter) ConfirmDeletion(required string) error {
	if _, err := fmt.Fprintf(p.out, "Type %q to confirm deletion: ", required); err != nil {
		return err
	}
	line, err := p.readLine()
	if err != nil {
		return err
	}
	if strings.TrimSpace(line) != required {
		return fmt.Errorf("confirmation %q did not match %q", strings.TrimSpace(line), required)
	}
	return nil
}

func (p *linePrompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
