package entity

import (
	"sort"
	"strconv"
	"strings"
)

// UCISection is one section of a UCI package as returned by
// `ubus call uci get {"config": pkg}`: named options plus list options.
type UCISection struct {
	Options map[string]string
	Lists   map[string][]string
	Name    string
	Type    string
	Index   int // position within the package, for ordered section types
}

// Opt returns a string option or def when absent.
func (s UCISection) Opt(name, def string) string {
	if v, ok := s.Options[name]; ok {
		return v
	}

	return def
}

// Bool returns a boolean option ("1", "on", "true", "yes", "enabled") or def when absent.
func (s UCISection) Bool(name string, def bool) bool {
	v, ok := s.Options[name]
	if !ok {
		return def
	}

	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes", "enabled":
		return true
	case "0", "off", "false", "no", "disabled":
		return false
	}

	return def
}

// Int returns an integer option or def when absent or malformed.
func (s UCISection) Int(name string, def int) int {
	v, ok := s.Options[name]
	if !ok {
		return def
	}

	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}

	return n
}

// List returns a list option. A plain `option` with the same name is
// accepted too and split on whitespace, which is how UCI itself treats it.
func (s UCISection) List(name string) []string {
	if l, ok := s.Lists[name]; ok {
		return l
	}

	if v, ok := s.Options[name]; ok && strings.TrimSpace(v) != "" {
		return strings.Fields(v)
	}

	return nil
}

// UCIPackage is a decoded UCI config file.
type UCIPackage struct {
	Sections map[string]UCISection
	Name     string
}

// Section returns a named section.
func (p UCIPackage) Section(name string) (UCISection, bool) {
	s, ok := p.Sections[name]

	return s, ok
}

// OfType returns every section of the given type ordered by position.
func (p UCIPackage) OfType(typ string) []UCISection {
	out := make([]UCISection, 0, len(p.Sections))

	for _, s := range p.Sections {
		if s.Type == typ {
			out = append(out, s)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })

	return out
}
