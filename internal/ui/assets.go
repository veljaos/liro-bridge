package ui

import "embed"

// Assets embeds every HTML/CSS/JS file the agent's windows serve over
// the virtual host mapping (F5 §2.4) — tokens.css, intents.css
// (scripts/synctokens' committed output), bridge.js, and each window's
// own page under pages/. Callers pass fs.Sub(ui.Assets, "assets") as
// Options.Assets so the virtual host root lines up with these files'
// own absolute references (e.g. "/pages/consent.html" referencing
// "/tokens.css").
//
//go:embed assets
var Assets embed.FS
