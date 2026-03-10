package assets

import _ "embed"

// ModelsYAML is the embedded models.yaml seed file.
// It contains default models, pricing, and context window sizes for all
// supported providers. The application also fetches a fresh copy from GitHub
// at runtime and caches it locally — this embedded version is the fallback.
//
//go:embed models.yaml
var ModelsYAML []byte
