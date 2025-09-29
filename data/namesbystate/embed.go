package namesbystate

import "embed"

// Files holds the embedded names-by-state dataset.
//
//go:embed *.TXT
var Files embed.FS
