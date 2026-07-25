// Package dependabot orchestrates the dependabot-sweep command: walk every
// named owner, find its open Dependabot pull requests, and merge the ones that
// are both green and non-major.
//
// The sweep exists because GitHub's native auto-merge is a paid feature for
// private repositories, so the fleet cannot rely on the platform to land
// dependency updates. Merging through the pull-request API carries no plan
// gate, which makes one mechanism correct for public and private repos alike.
//
// Three properties shape the design:
//
//   - Rate-aware. Discovery is one search request per owner rather than one
//     list request per repository, and the sweep re-reads the (unmetered) rate
//     budget as it goes, stopping cleanly at a floor instead of failing.
//   - Resilient. A failure against one pull request is recorded as a skip and
//     the sweep continues; no single repository can abort the run.
//   - Accountable. Every pull request the sweep declines carries a reason, so a
//     result can never read as "everything was handled" when it was not.
//
// Run delegates all GitHub I/O to internal/github and holds no CLI or
// output-formatting logic. This is the domain tier between the app tier
// (internal/app/commands/dependabot) and the implementation tier.
package dependabot
