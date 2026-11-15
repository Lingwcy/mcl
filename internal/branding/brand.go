// Package branding holds the product identity used by the native application.
package branding

import _ "embed"

const Name = "mcl"
const FullName = "Meow Craft Launcher"

//go:embed logo.png
var LogoPNG []byte

// Credit represents a contributor recognized in the launcher's acknowledgements.
type Credit struct {
	Name    string
	HeadPNG []byte
}

//go:embed heads/Ou_Takahiro.png
var ouTakahiroHeadPNG []byte

//go:embed heads/XiaoMouzOvO.png
var xiaoMouzOvOHeadPNG []byte

//go:embed heads/iskj.png
var iskjHeadPNG []byte

// Credits lists contributors recognized for mcl development and support.
var Credits = []Credit{
	{Name: "Ou_Takahiro", HeadPNG: ouTakahiroHeadPNG},
	{Name: "XiaoMouzOvO", HeadPNG: xiaoMouzOvOHeadPNG},
	{Name: "iskj", HeadPNG: iskjHeadPNG},
}
