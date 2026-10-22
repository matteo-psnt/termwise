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

// renderListRow renders one row of a picker list: the focused row gets the
// "▶ " marker and the Selected style, others are indented to line up. badge
// is appended unstyled so it keeps its own colors.
//
// The theme picker doesn't use this: it styles only the marker, so its
// colored swatches aren't recolored on the focused row.
func renderListRow(styles configStyles, focused bool, text, badge string) string {
	if focused {
		return styles.Selected.Render("▶ "+text) + badge
	}
	return styles.Normal.Render("  "+text) + badge
}

// listWindow returns the [start, end) slice of an n-item list that keeps
// cursor visible within size rows, scrolling only once the cursor nears an edge.
func listWindow(n, cursor, size int) (int, int) {
	if n <= size {
		return 0, n
	}
	start := min(max(cursor-size/2, 0), n-size)
	return start, start + size
}
