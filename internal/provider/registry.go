package provider

import "fmt"

// Factory constructs an AgentClient for a provider from a resolved Config.
type Factory func(Config) (AgentClient, error)

// Info is a registered built-in provider. Carries display metadata plus the
// factory used to construct a client.
type Info struct {
	Name          string
	DisplayName   string
	DefaultEnvVar string // empty for providers that need no API key (e.g. Ollama)
	Factory       Factory
}

var (
	registry = map[string]Info{}
	order    []string
)

// Register adds a built-in provider. Intended to be called from package init().
// Panics on duplicate registration — that's a programming error, not a runtime
// condition.
func Register(info Info) {
	if info.Name == "" {
		panic("provider.Register: Name is required")
	}
	if info.Factory == nil {
		panic("provider.Register: Factory is required for " + info.Name)
	}
	if _, exists := registry[info.Name]; exists {
		panic("provider.Register: duplicate registration for " + info.Name)
	}
	registry[info.Name] = info
	order = append(order, info.Name)
}

// Lookup returns the registered Info for a provider name.
func Lookup(name string) (Info, bool) {
	info, ok := registry[name]
	return info, ok
}

// Catalog returns all registered providers in registration order.
func Catalog() []Info {
	out := make([]Info, len(order))
	for i, name := range order {
		out[i] = registry[name]
	}
	return out
}

// Names returns the names of all registered providers in registration order.
func Names() []string {
	out := make([]string, len(order))
	copy(out, order)
	return out
}

// NewClient constructs a client for the named provider via its registered factory.
func NewClient(name string, cfg Config) (AgentClient, error) {
	info, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q — registered: %v", name, Names())
	}
	return info.Factory(cfg)
}
