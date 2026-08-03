package dependabot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bump
	}{
		{
			name: "single dependency within major",
			body: "Bumps [google.golang.org/grpc](https://github.com/grpc/grpc-go) from 1.81.0 to 1.82.1.",
			want: bumpWithinMajor,
		},
		{
			name: "single dependency crossing major",
			body: "Bumps [undici](https://github.com/nodejs/undici) from 5.29.0 to 7.28.0.",
			want: bumpMajor,
		},
		{
			name: "grouped, every transition within major",
			body: "Updates `vite` from 7.3.5 to 7.3.6\nUpdates `vitest` from 3.1.0 to 3.4.0\n",
			want: bumpWithinMajor,
		},
		{
			name: "grouped, one transition crosses major taints the whole pull request",
			body: "Updates `undici` from 5.29.0 to 7.28.0\nUpdates `wrangler` from 3.114.17 to 4.112.0\n",
			want: bumpMajor,
		},
		{
			name: "grouped, later transition crosses major",
			body: "Updates `vite` from 7.3.5 to 7.3.6\nUpdates `wrangler` from 3.1.0 to 4.0.0\n",
			want: bumpMajor,
		},
		{
			name: "zero-major minor bump stays within major",
			body: "Bumps golang.org/x/net from 0.53.0 to 0.55.0.",
			want: bumpWithinMajor,
		},
		{
			name: "no parseable transition",
			body: "Bumps undici and wrangler. These dependencies need to be updated together.",
			want: bumpUnknown,
		},
		{
			name: "empty body",
			body: "",
			want: bumpUnknown,
		},
		{
			name: "transition at end of body without trailing punctuation",
			body: "Updates `vite` from 7.3.5 to 7.3.6",
			want: bumpWithinMajor,
		},
		{
			name: "prerelease suffix does not change the major",
			body: "Bumps foo from 2.0.0-rc.1 to 2.0.0.",
			want: bumpWithinMajor,
		},
		{
			name: "major crossing with prerelease suffix",
			body: "Bumps foo from 1.9.0 to 2.0.0-rc.1.",
			want: bumpMajor,
		},
		{
			name: "multi-digit majors compare by value not prefix",
			body: "Bumps foo from 9.1.0 to 10.0.0.",
			want: bumpMajor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.New(t).Equal(tt.want, classify(pullBody(tt.body)))
		})
	}
}

func TestMajorOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version version
		want    major
	}{
		{version: "1.82.1", want: "1"},
		{version: "10.0.0", want: "10"},
		{version: "2", want: "2"},
		{version: "2-rc.1", want: "2"},
		{version: "3+build.5", want: "3"},
		{version: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.version), func(t *testing.T) {
			t.Parallel()
			assert.New(t).Equal(tt.want, majorOf(tt.version))
		})
	}
}

func TestPadded(t *testing.T) {
	t.Parallel()
	assert.New(t).Equal("body\n", padded("body"))
}

func TestClassifyIgnoresEmbeddedReleaseNotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bump
	}{
		{
			name: "upstream release notes carry foreign major bumps",
			body: "Bumps [fast-uri](https://github.com/fastify/fast-uri) from 3.1.0 to 3.1.4.\n" +
				"<details>\n<summary>Release notes</summary>\n" +
				"<li>bump actions/checkout from 4 to 5</li>\n" +
				"<li>bump actions/setup-node from 5 to 6</li>\n" +
				"</details>\n",
			want: bumpWithinMajor,
		},
		{
			name: "grouped pull request keeps every dependency outside the blocks",
			body: "Bumps undici and wrangler.\n\n" +
				"Updates `undici` from 5.29.0 to 7.28.0\n" +
				"<details><summary>notes</summary>irrelevant from 1.0.0 to 1.0.1</details>\n" +
				"<br />\n\n" +
				"Updates `wrangler` from 3.114.17 to 4.112.0\n" +
				"<details><summary>notes</summary>more from 2.0.0 to 2.0.1</details>\n",
			want: bumpMajor,
		},
		{
			name: "nested blocks do not resume capture early",
			body: "Bumps foo from 1.0.0 to 1.1.0.\n" +
				"<details><summary>outer</summary>\n" +
				"<details><summary>inner</summary>bump bar from 1 to 9</details>\n" +
				"still inside from 2 to 8\n" +
				"</details>\n",
			want: bumpWithinMajor,
		},
		{
			name: "unbalanced block drops the remainder rather than trusting it",
			body: "Bumps foo from 1.0.0 to 1.1.0.\n<details>\nbump bar from 1 to 9\n",
			want: bumpWithinMajor,
		},
		{
			name: "body that is only an unclosed block yields no transition",
			body: "<details>\nBumps foo from 1.0.0 to 2.0.0.\n",
			want: bumpUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.New(t).Equal(tt.want, classify(pullBody(tt.body)))
		})
	}
}

func TestStripDetails(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	want.Equal("keep", stripDetails("keep"), "a body with no blocks passes through untouched")
	want.Equal("a\nb", stripDetails("a<details>gone</details>b"), "text on both sides survives, the block does not")
	want.Equal("a", stripDetails("a<details>gone"), "an unclosed block drops its remainder")
	want.NotContains(stripDetails("x<details>y<details>z</details>w</details>v"), "z")
}

// TestClassifyBumpNotForwardDeclinesWhatIsNotAnAdvance pins the direction rule
// with the shape that motivated it: Dependabot proposing to replace a floating
// internal action reference with a pinned version. The body reads "from 2 to
// 2.6.1", which stays inside major 2 and so passed the old within-major gate —
// but "2" is a moving tag that already resolved to a NEWER release, making the
// merge a silent downgrade of every repository's CI.
func TestClassifyBumpNotForwardDeclinesWhatIsNotAnAdvance(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body string
		want bump
	}{
		{
			name: "pinning a floating major tag is unorderable",
			body: "Bumps [nicerobot/tools.build](https://github.com/nicerobot/tools.build) " +
				"from 2 to 2.6.1 in the github-actions group\n",
			want: bumpNotForward,
		},
		{
			name: "an outright downgrade inside a major",
			body: "Bumps foo from 2.11.0 to 2.6.1.",
			want: bumpNotForward,
		},
		{
			name: "standing still is not an advance",
			body: "Bumps foo from 1.2.0 to 1.2.0.",
			want: bumpNotForward,
		},
		{
			name: "a release regressing into a pre-release",
			body: "Bumps foo from 2.0.0 to 2.0.0-rc.1.",
			want: bumpNotForward,
		},
		{
			name: "one bad transition condemns a grouped pull request",
			body: "Updates `alpha` from 1.2.0 to 1.3.0\nUpdates `beta` from 1.9.0 to 1.4.0\n",
			want: bumpNotForward,
		},
		{
			name: "an ordinary patch bump still advances",
			body: "Bumps foo from 1.1.0 to 1.1.4.",
			want: bumpWithinMajor,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classify(pullBody(tt.body)))
		})
	}
}

// TestTransitionReCapturesWholeVersions pins the capture the direction rule
// depends on: a version runs to whitespace and keeps its own dots. Reading only
// as far as the first dot yielded "1" for a target of 1.2.0, which left every
// comparison here blind to everything below the major.
func TestTransitionReCapturesWholeVersions(t *testing.T) {
	t.Parallel()

	matches := transitionRe.FindAllStringSubmatch("Bumps foo from 1.1.0 to 1.2.0.\n", -1)
	require.Len(t, matches, 1)
	assert.Equal(t, version("1.1.0"), trimmed(capture(matches[0][1])))
	assert.Equal(
		t,
		version("1.2.0"),
		trimmed(capture(matches[0][2])),
		"the target version is read whole, not truncated",
	)
}

// TestMovesForwardAndPrecedesOrderReleases names the two helpers the direction
// rule rests on: movesForward vouches for a transition only when both sides are
// complete versions and the second is later, and precedes puts a pre-release
// before the release it leads to while refusing to order one pre-release
// against another.
func TestMovesForwardAndPrecedesOrderReleases(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.True(movesForward("1.2.0", "1.2.1"))
	want.False(movesForward("1.2.1", "1.2.0"), "backwards is not forward")
	want.False(movesForward("2", "2.6.1"), "an incomplete side cannot be ordered")
	want.False(movesForward("1.2.0", "1.2"), "an incomplete target cannot be ordered")
	want.False(movesForward("1.x.0", "1.2.0"), "a non-numeric component cannot be ordered")

	rc, ok := complete("2.0.0-rc.1")
	require.True(t, ok)
	final, ok := complete("2.0.0+build.7")
	require.True(t, ok)
	want.True(precedes(rc, final), "a pre-release precedes its release, and build metadata is ignored")
	want.False(precedes(final, rc))
	want.False(precedes(final, final), "a version does not precede itself")

	other, ok := complete("2.0.0-rc.2")
	require.True(t, ok)
	want.False(precedes(rc, other), "one pre-release is never ordered against another here")
}
