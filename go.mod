module github.com/nicerobot/tools.admin

go 1.26.4

require (
	github.com/gomatic/go-error v0.3.15
	github.com/stretchr/testify v1.12.0
	github.com/urfave/cli/v3 v3.10.1
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/kr/text v0.2.0 // indirect

// v0.3.0 and v0.4.0 carried fleet owner and account names in a workflow file and
// test fixtures. The names are removed from the repository, but proxy.golang.org
// is append-only, so those module zips remain fetchable by explicit version.
// Retracting them makes `go get` refuse the versions and stops them being
// selected; removal from the mirror is a separate request to the Go team.
retract (
	v0.4.0 // published fleet owner and account names
	v0.3.0 // published fleet owner and account names
)
