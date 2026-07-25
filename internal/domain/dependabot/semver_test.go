package dependabot

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
