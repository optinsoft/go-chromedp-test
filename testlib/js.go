package testlib

import (
	_ "embed"
)

var (
	// visibleJS is a javascript snippet that returns true or false depending on if
	// the specified node's offsetWidth, offsetHeight or getClientRects().length is
	// not null.
	//go:embed js/visible.js
	visibleJS string
)
