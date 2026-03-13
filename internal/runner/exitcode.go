package runner

// ExitCode is a sentinel error that carries a specific process exit code.
// It is never printed — the caller uses the Code directly.
type ExitCode struct {
	Code int
}

func (e ExitCode) Error() string { return "" }
