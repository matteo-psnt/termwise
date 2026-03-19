package assets

import _ "embed"

// ModelsYAML is the embedded models.yaml seed file.
// It contains default models, pricing, and context window sizes for all
// supported providers. The application also fetches a fresh copy from GitHub
// at runtime and caches it locally — this embedded version is the fallback.
//
//go:embed models.yaml
var ModelsYAML []byte

// AllowlistYAML is the embedded allowlist.yaml file.
// It contains the built-in set of commands that are auto-approved without
// user confirmation. Users can extend this via shell.allow in config.toml.
//
//go:embed allowlist.yaml
var AllowlistYAML []byte
