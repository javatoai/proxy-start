// Package assets provides immutable copies of the shared product artwork.
package assets

import _ "embed"

//go:generate go run ../cmd/generate-icons -output .

//go:embed proxy-start.svg
var logoSVG string

//go:embed proxy-start-32.png
var iconPNG string

// LogoSVG is the single vector source used by the UI and icon generator.
func LogoSVG() []byte { return []byte(logoSVG) }

// IconPNG returns the generated 32px native window and notification-tray icon.
func IconPNG() []byte { return []byte(iconPNG) }
