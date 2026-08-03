// Package domain is the root vocabulary of the domain tier: the concepts every
// command's domain package speaks, independent of any one verb.
//
// It declares no Config, Result, or Run, so it is not itself a command — the
// domain-tier contract analyzer skips it.
package domain

// Argument is one positional CLI argument. It is an alias, not a defined type:
// the runner contract fixes the variadic element type to string, and the alias
// names the domain concept without changing the signature's identity. Every
// domain Run spells its variadic ...domain.Argument so the concept has one name
// across the whole tier.
type Argument = string
