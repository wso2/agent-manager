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

	stdinIsTerminal  bool
	stderrIsTerminal bool
}

func System() *IOStreams {
	return &IOStreams{
		In:               os.Stdin,
		Out:              os.Stdout,
		ErrOut:           os.Stderr,
		stdinIsTerminal:  term.IsTerminal(int(os.Stdin.Fd())),
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

func (s *IOStreams) SetTerminal(stdin, stderr bool) {
	s.stdinIsTerminal = stdin
	s.stderrIsTerminal = stderr
}
