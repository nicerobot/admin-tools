package dependabot

import (
	"regexp"
	"strings"
)

// transitionRe matches the version transitions Dependabot writes into a pull
// request body. Both shapes it emits carry the same "from X to Y" tail:
//
//	Bumps [google.golang.org/grpc](...) from 1.81.0 to 1.82.1.
//	Updates `undici` from 5.29.0 to 7.28.0
//
// A grouped pull request repeats the line once per dependency, so a single body
// yields every transition the merge would apply.
var transitionRe = regexp.MustCompile(`from ([0-9][^\s]*?) to ([0-9][^\s]*?)[\s.,)]`)

// pullBody is the prose of a Dependabot pull request, the sole reliable source
// of its version transitions. The commit trailer carries dependency-name and
// dependency-version but not the prior version, so the bump size cannot be
// derived from it.
type pullBody string

// bump classifies a pull request's version change.
type bump int

const (
	// bumpUnknown means no transition could be parsed, so the change cannot be
	// judged and must not be merged automatically.
	bumpUnknown bump = iota
	// bumpWithinMajor means every transition stays inside its major version.
	bumpWithinMajor
	// bumpMajor means at least one transition crosses a major boundary.
	bumpMajor
)

// classify reports the largest bump the body describes. A body with no parseable
// transition is bumpUnknown: unjudgeable is never treated as safe.
func classify(body pullBody) bump {
	matches := transitionRe.FindAllStringSubmatch(padded(pullBody(stripDetails(body))), -1)
	if len(matches) == 0 {
		return bumpUnknown
	}
	for _, m := range matches {
		if majorOf(version(m[1])) != majorOf(version(m[2])) {
			return bumpMajor
		}
	}
	return bumpWithinMajor
}

// padded appends a terminator so a transition ending at the very end of the
// body still presents the trailing delimiter the pattern requires.
func padded(body pullBody) string { return string(body) + "\n" }

// detailsRe matches either half of a <details> element.
var detailsRe = regexp.MustCompile(`</?details>`)

// stripDetails returns only the prose Dependabot wrote itself, with every
// <details> block removed.
//
// This is load-bearing, not cosmetic. Dependabot embeds the upstream project's
// release notes inside <details>, and those notes contain the upstream's own
// dependency bumps ("bump actions/checkout from 4 to 5"). Scanning the whole
// body reads those foreign transitions as this pull request's own and
// misclassifies a security patch as a major bump.
//
// Blocks nest, and a grouped pull request interleaves them with the per-
// dependency "Updates X from A to B" lines, so truncating at the first
// <details> would drop later dependencies — turning a real major into an
// apparent minor. Only depth-aware removal is correct here.
func stripDetails(body pullBody) string {
	raw := rawBody(body)
	s := scan{}
	for _, loc := range detailsRe.FindAllStringIndex(string(raw), -1) {
		s = s.step(raw, loc)
	}
	return strings.Join(s.tail(raw), "\n")
}

// rawBody is a pull-request body as flat text, the subject of a stripping walk.
type rawBody string

// scan is the depth-tracking state of a <details>-stripping walk. It is an
// immutable value: each step returns the next state.
type scan struct {
	kept   []string
	depth  int
	cursor int
}

// step folds one <details> or </details> occurrence into the walk, keeping the
// text that precedes it only when that text sits outside every block.
func (s scan) step(raw rawBody, loc []int) scan {
	if s.depth == 0 {
		s.kept = append(s.kept, string(raw[s.cursor:loc[0]]))
	}
	if isClosing(raw, loc) {
		return s.close(loc)
	}
	s.depth++
	return s
}

// close descends one level, resuming capture once the outermost block ends.
func (s scan) close(loc []int) scan {
	s.depth--
	if s.depth <= 0 {
		s.depth, s.cursor = 0, loc[1]
	}
	return s
}

// tail appends the text after the final block. An unbalanced body leaves depth
// above zero, so the remainder is dropped rather than trusted — fewer parsed
// transitions can only make classify more conservative, never less.
func (s scan) tail(raw rawBody) []string {
	if s.depth != 0 {
		return s.kept
	}
	return append(s.kept, string(raw[s.cursor:]))
}

// isClosing reports whether the matched tag is the closing half.
func isClosing(raw rawBody, loc []int) bool { return raw[loc[0]+1] == '/' }

// version is a single dependency version as it appears in a pull request body.
type version string

// major is the leading component of a version, the part a bump must preserve.
type major string

// majorOf returns the leading major-version component of a version. Anything
// before the first separator is the major; a version with no separator is its
// own major.
func majorOf(v version) major {
	if idx := strings.IndexAny(string(v), ".-+"); idx >= 0 {
		return major(v[:idx])
	}
	return major(v)
}
