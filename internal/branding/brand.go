// Package branding holds the product identity used by the native application.
package branding

import _ "embed"

const Name = "mcl"
const FullName = "Meow Craft Launcher"

//go:embed logo.png
var LogoPNG []byte
