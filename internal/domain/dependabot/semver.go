package dependabot

import (
	"regexp"
	"strconv"
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
//
// Each version runs to whitespace and is then trimmed of sentence punctuation
// by [trimmed]. Matching lazily up to a delimiter instead would stop at the
// version's OWN first dot — "to 1.2.0." yields "1", which silently reduced
// every comparison here to leading digits and left the real target version
// unread.
var transitionRe = regexp.MustCompile(`from ([0-9]\S*) to ([0-9]\S*)`)

// pullBody is the prose of a Dependabot pull request, the sole reliable source
// of its version transitions. The commit trailer carries dependency-name and
// dependency-version but not the prior version, which leaves the bump size
// underivable from it.
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
	// bumpNotForward means at least one transition is not provably a move
	// forward: it goes backward, stands still, or starts from a reference that
	// cannot be ordered at all. Staying inside a major says nothing about
	// direction, so this is classified apart from it.
	bumpNotForward
)

// classify reports the largest bump the body describes. A body with no parseable
// transition is bumpUnknown: unjudgeable is never treated as safe.
//
// Crossing a major boundary outranks direction, because a major bump is the
// louder fact and is already gated behind explicit consent; among transitions
// that stay inside their major, one that does not provably move forward
// condemns the whole pull request.
func classify(body pullBody) bump {
	matches := transitionRe.FindAllStringSubmatch(padded(pullBody(stripDetails(body))), -1)
	if len(matches) == 0 {
		return bumpUnknown
	}
	result := bumpWithinMajor
	for _, m := range matches {
		from, to := trimmed(capture(m[1])), trimmed(capture(m[2]))
		if majorOf(from) != majorOf(to) {
			return bumpMajor
		}
		if !movesForward(from, to) {
			result = bumpNotForward
		}
	}
	return result
}

// movesForward reports whether a transition provably advances.
//
// Both sides must be COMPLETE versions. A bare major like "2" is not a version
// but a floating reference — the fleet's own CI actions are pinned that way, so
// the tag is moved to each release and "2" may already denote something newer
// than any fixed version the prose names. Ordering it against "2.6.1" would
// read the string as smaller and call a downgrade an upgrade — the way a
// proposal to pin an up-to-date float passes for progress. Nothing about
// the current target is derivable from the body, so an incomplete side is
// declared unorderable and declined rather than guessed at.
func movesForward(from, to version) bool {
	before, ok := complete(from)
	if !ok {
		return false
	}
	after, ok := complete(to)
	if !ok {
		return false
	}
	return precedes(before, after)
}

// release is a version's ordered form: its numeric components, most
// significant first, and whether a pre-release suffix follows them.
type release struct {
	components   []int
	isPrerelease bool
}

// complete parses a version into its ordered form, reporting false unless all
// three of major, minor and patch are present and numeric. Build metadata is
// discarded — semver excludes it from precedence entirely.
func complete(v version) (release, bool) {
	withoutBuild, _, _ := strings.Cut(string(v), "+")
	numbers, pre, hasPre := strings.Cut(withoutBuild, "-")
	fields := strings.Split(numbers, ".")
	if len(fields) != 3 {
		return release{}, false
	}
	parsed := make([]int, 0, len(fields))
	for _, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return release{}, false
		}
		parsed = append(parsed, number)
	}
	return release{components: parsed, isPrerelease: hasPre && pre != ""}, true
}

// precedes reports whether before sorts strictly earlier than after.
//
// Numeric components decide it whenever they differ. When they match, semver
// puts a pre-release BEFORE the release it leads to (2.0.0-rc.1 precedes
// 2.0.0), so shedding a suffix advances. Everything else at equal components —
// the same version twice, a release regressing into a pre-release, or one
// pre-release to another — is not an advance this gate will vouch for:
// ordering pre-release identifiers is a rule of its own, and a transition that
// needs it deserves a human rather than a guess.
func precedes(before, after release) bool {
	for i := range before.components {
		if before.components[i] != after.components[i] {
			return before.components[i] < after.components[i]
		}
	}
	return before.isPrerelease && !after.isPrerelease
}

// capture is one version as the transition pattern matched it, sentence
// punctuation and all.
type capture string

// trimmed reads a captured version, dropping the sentence punctuation that
// follows it in prose ("to 1.2.0." and "to 1.2.0)" both name 1.2.0).
func trimmed(captured capture) version {
	return version(strings.TrimRight(string(captured), ".,);:"))
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
