package configui

// moveListCursor applies an up/down (or k/j) key to the cursor of an n-item
// list, stopping at either end. ok reports whether key was a movement key, so
// callers can handle it before their own key switch.
func moveListCursor(key string, cursor, n int) (next int, ok bool) {
	switch key {
	case "up", "k":
		if cursor > 0 {
			cursor--
		}
		return cursor, true
	case "down", "j":
		if cursor < n-1 {
			cursor++
		}
		return cursor, true
	}
	return cursor, false
}
