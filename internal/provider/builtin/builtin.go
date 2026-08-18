// Package builtin blank-imports every built-in provider so their init()
// registrations run. Import this package once from main to make the registry
// populated. The import order here also fixes the catalog display order.
package builtin

import (
	_ "github.com/matteo-psnt/termwise/internal/provider/anthropic"
	_ "github.com/matteo-psnt/termwise/internal/provider/openaicompat"
)
