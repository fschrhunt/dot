// Package dot bundles the starter setup so init works from a lone binary.
package dot

import "embed"

// Example contains the setup copied by dot init, relative to example/.
//
//go:embed example
var Example embed.FS
