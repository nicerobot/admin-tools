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
	matches := transitionRe.FindAllStringSubmatch(padded(body), -1)
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
