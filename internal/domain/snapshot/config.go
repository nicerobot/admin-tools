package snapshot

import "github.com/nicerobot/tools.admin/internal/repo"

// Config holds the flags for the snapshot command. Its fields are bound by the
// CLI tier and read by Run; it carries no behavior. The owner is a positional
// argument, not a flag, so it reaches Run through the arguments rather than
// this Config. The field reuses the implementation tier's named type, so no
// domain-local types.go is needed.
type Config struct {
	SettingsPath repo.SettingsPath
}
