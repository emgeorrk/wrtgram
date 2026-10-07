package tgtext

import "strings"

// Split cuts text into chunks of at most limit characters on line boundaries.
// A chunk that ends inside a <pre> block gets the block closed and the next
// one re-opens it, so every chunk is valid HTML on its own.
func Split(text string, limit int) []string {
	if limit <= 0 {
		limit = MaxMessage
	}

	if len([]rune(text)) <= limit {
		return []string{text}
	}

	s := &splitter{limit: limit}
	pieceMax := limit - len(preOpen) - len(preClose)

	for _, line := range strings.SplitAfter(text, "\n") {
		for _, piece := range hardCut(line, pieceMax) {
			s.add(piece)
		}
	}

	s.flush()

	return s.chunks
}

type splitter struct {
	chunks  []string
	cur     strings.Builder
	limit   int
	curLen  int
	openPre bool
}

// add appends a piece that is guaranteed to fit into an empty chunk.
func (s *splitter) add(piece string) {
	n := len([]rune(piece))
	if s.curLen+n > s.limit-len(preClose) {
		s.flush()
	}

	s.cur.WriteString(piece)

	s.curLen += n
	s.openPre = trackPre(piece, s.openPre)
}

// flush closes the current chunk; inside a <pre> block it appends the closing
// tag and starts the next chunk with an opening one.
func (s *splitter) flush() {
	text := s.cur.String()
	s.cur.Reset()

	s.curLen = 0

	if text == "" {
		return
	}

	if s.openPre {
		text += preClose

		s.cur.WriteString(preOpen)

		s.curLen = len(preOpen)
	}

	s.chunks = append(s.chunks, text)
}

// hardCut splits a single line longer than limit characters.
func hardCut(line string, limit int) []string {
	r := []rune(line)
	if len(r) <= limit {
		return []string{line}
	}

	var out []string
	for len(r) > limit {
		out = append(out, string(r[:limit]))
		r = r[limit:]
	}

	return append(out, string(r))
}

// trackPre reports whether we are inside a <pre> block after s.
func trackPre(s string, open bool) bool {
	for {
		if open {
			i := strings.Index(s, preClose)
			if i < 0 {
				return true
			}

			s = s[i+len(preClose):]
			open = false

			continue
		}

		i := strings.Index(s, preOpen)
		if i < 0 {
			return false
		}

		s = s[i+len(preOpen):]
		open = true
	}
}
