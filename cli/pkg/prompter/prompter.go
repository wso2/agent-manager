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
	"strconv"
	"strings"
)

type Prompter interface {
	ConfirmDeletion(required string) error
	Confirm(prompt string) (bool, error)
}

// Selector asks the user to pick one of a few options; kept apart from Prompter so the
// commands that only confirm don't have to fake it in their tests.
type Selector interface {
	// Select returns the index of the chosen option; an empty answer picks defaultIndex.
	Select(prompt string, options []string, defaultIndex int) (int, error)
}

// selectAttempts bounds how often Select asks again after an answer it can't use.
const selectAttempts = 3

type linePrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) Prompter {
	return &linePrompter{in: bufio.NewReader(in), out: out}
}

// NewSelector returns a Selector that numbers the options and reads the choice from in.
func NewSelector(in io.Reader, out io.Writer) Selector {
	return &linePrompter{in: bufio.NewReader(in), out: out}
}

func (p *linePrompter) Select(prompt string, options []string, defaultIndex int) (int, error) {
	if _, err := fmt.Fprintln(p.out, prompt); err != nil {
		return 0, err
	}
	for i, o := range options {
		if _, err := fmt.Fprintf(p.out, "  %d) %s\n", i+1, o); err != nil {
			return 0, err
		}
	}
	for attempt := range selectAttempts {
		if _, err := fmt.Fprintf(p.out, "Choose 1-%d [%d]: ", len(options), defaultIndex+1); err != nil {
			return 0, err
		}
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return 0, err
		}
		answer := strings.TrimSpace(line)
		// Input that ends after a bad answer is no choice, not a request for the default.
		if answer == "" && (err == nil || attempt == 0) {
			return defaultIndex, nil
		}
		if n, convErr := strconv.Atoi(answer); convErr == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		if err == io.EOF {
			break
		}
		if _, werr := fmt.Fprintf(p.out, "%q is not one of the choices.\n", answer); werr != nil {
			return 0, werr
		}
	}
	return 0, fmt.Errorf("no valid choice for %q", prompt)
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

func (p *linePrompter) Confirm(prompt string) (bool, error) {
	if _, err := fmt.Fprintf(p.out, "%s [y/N]: ", prompt); err != nil {
		return false, err
	}
	line, err := p.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func (p *linePrompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
