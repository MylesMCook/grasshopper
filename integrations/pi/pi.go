// Package pi holds Grasshopper's Pi extension. The same file is the Pi
// package's extension (see the repository's package.json) and the template
// the client writes into Pi's extensions folder.
package pi

import _ "embed"

// Extension is the extension source. ConfigLine is the line the client
// replaces with its configuration path when it writes the extension.
//
//go:embed extension.ts
var Extension string

// ConfigLine names the client configuration in the extension source.
const ConfigLine = `const writtenConfig: string = "";`
