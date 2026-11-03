// Package locales contains the built-in translations.
package locales

import "embed"

//go:embed en.json zh.json ja.json
var Files embed.FS
