package config

// Blank-import the provider builtins so their init() registrations run inside
// the config-package test binary. Production code wires this in main.
import _ "github.com/matteo-psnt/termwise/internal/provider/builtin"
