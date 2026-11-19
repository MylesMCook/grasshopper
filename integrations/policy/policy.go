// Package policy holds the shared memory policy that every agent loads at
// session start.
package policy

import _ "embed"

// Text is the canonical memory policy. The client serves this copy instead of
// the file written at connect time, so a policy change reaches every machine
// with the client's self-update.
//
//go:embed AGENTS.md
var Text string
