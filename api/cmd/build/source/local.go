package source

import (
	"io"
	"os"
)

type SourceLocal struct{}

func (s *SourceLocal) Fetch(out io.Writer) (string, error) {
	return os.Getwd()
}
