package execx

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

var errFixtureSyntax = errors.New("fixture syntax")

// Fixture file format, one block per command:
//
//	>>> ubus call system board
//	{ ...stdout... }
//	>>>stream logread -f
//	line one
//	line two
//	>>>error ip route del 0.0.0.0/1
//	>>>path awg 1
//
// A header starts with ">>>"; the body runs until the next header. The
// trailing newline of a body is dropped.
const (
	hdrRun    = ">>> "
	hdrStream = ">>>stream "
	hdrError  = ">>>error "
	hdrPath   = ">>>path "
)

// LoadFixtures builds a Fake from a fixture file.
func LoadFixtures(path string) (*Fake, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fixtures: %w", err)
	}
	defer f.Close()

	p := &fixtureParser{fake: NewFake()}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLine)

	for sc.Scan() {
		if err := p.line(sc.Text()); err != nil {
			return nil, err
		}
	}

	p.flush()

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("fixtures: %w", err)
	}

	return p.fake, nil
}

type fixtureParser struct {
	fake *Fake
	kind string
	key  string
	body []string
}

func (p *fixtureParser) line(line string) error {
	if !strings.HasPrefix(line, ">>>") {
		p.body = append(p.body, line)

		return nil
	}

	p.flush()

	for _, kind := range []string{hdrStream, hdrError, hdrRun} {
		if strings.HasPrefix(line, kind) {
			p.kind, p.key = kind, strings.TrimSpace(strings.TrimPrefix(line, kind))

			return nil
		}
	}

	if strings.HasPrefix(line, hdrPath) {
		name, exists, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, hdrPath)), " ")
		p.fake.Path(name, exists == "1")

		return nil
	}

	return fmt.Errorf("%w: %q", errFixtureSyntax, line)
}

// flush registers the block collected so far and resets the parser.
func (p *fixtureParser) flush() {
	text := strings.Join(p.body, "\n")

	switch p.kind {
	case hdrRun:
		p.fake.OnString(p.key, text)
	case hdrStream:
		p.fake.OnStream(p.key, p.body)
	case hdrError:
		p.fake.OnError(p.key, fmt.Errorf("%w: %s", ErrExit, text))
	}

	p.kind, p.key, p.body = "", "", nil
}
