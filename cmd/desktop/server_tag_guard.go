//go:build server || mcp

package main

// The desktop shell serves server.DesktopHandler, which treats every request
// as signed in, through the Wails asset handler. Wails's "server" build tag
// turns that handler into a network HTTP server, and its "mcp" tag opens a
// network endpoint that can run script in the window, so refuse to build
// with either.
var _ = cmdDesktopMustNotBeBuiltWithWailsServerOrMCPTags
