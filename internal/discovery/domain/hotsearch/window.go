package hotsearch

import "errors"

// Window is a Google Trends observation window.
type Window string

// Supported observation windows, shortest to longest.
const (
	Window7D  Window = "7D"
	Window30D Window = "30D"
	Window90D Window = "90D"
	Window12M Window = "12M"
	Window5Y  Window = "5Y"
)

// ErrInvalidWindow rejects unknown windows.
var ErrInvalidWindow = errors.New("keyword: invalid trend window")

// ParseWindow accepts the canonical window names.
func ParseWindow(raw string) (Window, error) {
	w := Window(raw)
	if !w.Valid() {
		return "", ErrInvalidWindow
	}
	return w, nil
}

// Valid reports whether the window is supported.
func (w Window) Valid() bool {
	switch w {
	case Window7D, Window30D, Window90D, Window12M, Window5Y:
		return true
	default:
		return false
	}
}

// AllWindows lists windows from shortest to longest for stable rendering.
func AllWindows() []Window {
	return []Window{Window7D, Window30D, Window90D, Window12M, Window5Y}
}
