// Package web contains the embedded management page.
package web

import "embed"

//go:embed index.html style.css app.js model.js i18n.js
var Files embed.FS
