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
