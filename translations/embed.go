package translations

import "embed"

// Files keeps the bundled translations available even when a host bind mount
// is empty or points to the wrong directory.
//
//go:embed *.json
var Files embed.FS
