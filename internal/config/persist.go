package config

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// spliceConfig returns old — the bytes of the user's config file — edited so
// that it decodes to cfg, touching only the keys whose values changed. The
// user's comments, formatting, key order and keys termwise does not know
// stay as they were; re-marshaling the struct would drop all of them.
//
// Both the old and the new config are marshaled first, so values are
// compared and written in one canonical encoding whatever quoting the user
// chose. The result is decoded again before it is returned: any construct
// the edits cannot handle (dotted keys, inline tables, arrays of tables)
// shows up as an error or a mismatch, and the caller falls back to a full
// rewrite rather than saving something wrong.
func spliceConfig(old []byte, cfg FileConfig) ([]byte, error) {
	var oldCfg FileConfig
	if err := toml.Unmarshal(old, &oldCfg); err != nil {
		return nil, fmt.Errorf("existing file does not parse: %w", err)
	}
	oldCanon, err := toml.Marshal(oldCfg)
	if err != nil {
		return nil, err
	}
	newCanon, err := toml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(oldCanon, newCanon) {
		return old, nil
	}

	file, err := indexTOML(old)
	if err != nil {
		return nil, err
	}
	was, err := indexTOML(oldCanon)
	if err != nil {
		return nil, err
	}
	want, err := indexTOML(newCanon)
	if err != nil {
		return nil, err
	}

	s := splicer{file: file, eol: "\n"}
	if bytes.Contains(old, []byte("\r\n")) {
		s.eol = "\r\n"
	}
	if err := s.plan(was, want); err != nil {
		return nil, err
	}
	out, err := s.apply()
	if err != nil {
		return nil, err
	}

	var got FileConfig
	if err := toml.Unmarshal(out, &got); err != nil {
		return nil, fmt.Errorf("edited file does not parse: %w", err)
	}
	gotCanon, err := toml.Marshal(got)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(gotCanon, newCanon) {
		return nil, errors.New("edited file does not match the new config")
	}
	return out, nil
}

// tomlKV is one `key = value` line found in a document.
type tomlKV struct {
	path []string
	// start and end span the whole line, trailing comment and newline
	// included, so deleting [start, end) leaves no trace of it.
	start, end int
	// valStart and valEnd span the value; valStart is -1 when the value is
	// not a scalar that can be replaced in place.
	valStart, valEnd int
	// keyText is the key as written, line is the line without its newline.
	keyText, line string
}

// tomlTable is one [table] header and the section that follows it.
type tomlTable struct {
	path  []string
	array bool
	// start is where the header line begins; lastEnd is the end of the
	// section's last key line (or of the header when it has none).
	start, lastEnd int
	header         string
	kvs            []*tomlKV
}

// tomlIndex is what the splicer needs to know about a document: where each
// plain key and each table is. Keys written dotted or inside an inline table
// are left out, so edits to them fail verification and fall back.
type tomlIndex struct {
	data        []byte
	kvs         map[string]*tomlKV
	tables      map[string]*tomlTable
	tableOrder  []*tomlTable
	rootKVs     []*tomlKV
	firstHeader int
}

func pathKey(p []string) string { return strings.Join(p, "\x00") }

func indexTOML(data []byte) (*tomlIndex, error) {
	idx := &tomlIndex{
		data:        data,
		kvs:         map[string]*tomlKV{},
		tables:      map[string]*tomlTable{},
		firstHeader: -1,
	}
	p := unstable.Parser{KeepComments: true}
	p.Reset(data)
	var cur *tomlTable
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			var path []string
			first, last := -1, 0
			it := n.Key()
			for it.Next() {
				k := it.Node()
				path = append(path, string(k.Data))
				if first < 0 {
					first = int(k.Raw.Offset)
				}
				last = int(k.Raw.Offset + k.Raw.Length)
			}
			start := lineStart(data, first)
			end := lineEnd(data, last)
			t := &tomlTable{
				path:    path,
				array:   n.Kind == unstable.ArrayTable,
				start:   start,
				lastEnd: end,
				header:  strings.TrimRight(string(data[start:end]), "\r\n"),
			}
			if idx.firstHeader < 0 {
				idx.firstHeader = start
			}
			if prev, ok := idx.tables[pathKey(path)]; ok {
				// Only an array of tables may repeat; mark it so no edit
				// is attempted inside it.
				prev.array = true
			} else {
				idx.tables[pathKey(path)] = t
			}
			idx.tableOrder = append(idx.tableOrder, t)
			cur = t
		case unstable.KeyValue:
			var keys []string
			var keyText string
			keyEnd := 0
			it := n.Key()
			for it.Next() {
				k := it.Node()
				keys = append(keys, string(k.Data))
				keyText = string(p.Raw(k.Raw))
				keyEnd = int(k.Raw.Offset + k.Raw.Length)
			}
			start := lineStart(data, int(n.Raw.Offset))
			end := lineEnd(data, int(n.Raw.Offset+n.Raw.Length))
			if cur != nil {
				cur.lastEnd = end
			}
			if len(keys) != 1 || (cur != nil && cur.array) {
				continue
			}
			kv := &tomlKV{
				start:    start,
				end:      end,
				valStart: -1,
				keyText:  keyText,
				line:     strings.TrimRight(string(data[start:end]), "\r\n"),
			}
			// The value runs from after the "=" to the end of the
			// expression; not every value node carries its own range.
			switch n.Value().Kind {
			case unstable.String, unstable.Bool, unstable.Integer, unstable.Float:
				exprEnd := int(n.Raw.Offset + n.Raw.Length)
				if eq := bytes.IndexByte(data[keyEnd:exprEnd], '='); eq >= 0 {
					vs := keyEnd + eq + 1
					for vs < exprEnd && (data[vs] == ' ' || data[vs] == '\t') {
						vs++
					}
					kv.valStart, kv.valEnd = vs, exprEnd
				}
			}
			if cur == nil {
				kv.path = keys
				idx.rootKVs = append(idx.rootKVs, kv)
			} else {
				kv.path = append(append([]string{}, cur.path...), keys...)
				cur.kvs = append(cur.kvs, kv)
			}
			idx.kvs[pathKey(kv.path)] = kv
		}
	}
	if err := p.Error(); err != nil {
		return nil, err
	}
	return idx, nil
}

// lineStart returns the offset of the start of the line containing off.
func lineStart(data []byte, off int) int {
	return bytes.LastIndexByte(data[:off], '\n') + 1
}

// lineEnd returns the offset just past the newline ending the line that
// contains off, or len(data) for a last line with no newline.
func lineEnd(data []byte, off int) int {
	if i := bytes.IndexByte(data[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(data)
}

// edit replaces data[start:end] with text; start == end is an insertion.
type edit struct {
	start, end int
	text       string
	seq        int
}

type splicer struct {
	file  *tomlIndex
	eol   string
	edits []edit
	// newTables holds tables to append at the end of the file, in the
	// order the canonical encoding lists them.
	newTables []*newTable
}

type newTable struct {
	path   []string
	header string
	lines  []string
}

func (s *splicer) add(start, end int, text string) {
	s.edits = append(s.edits, edit{start: start, end: end, text: text, seq: len(s.edits)})
}

// plan works out the edits that turn the file from was into want, both
// canonical encodings of the old and new config.
func (s *splicer) plan(was, want *tomlIndex) error {
	f := s.file

	// A provider removed from the map takes its whole section with it,
	// unknown keys and all: they belonged to that provider.
	var dropped [][]string
	for _, t := range was.tableOrder {
		if _, ok := want.tables[pathKey(t.path)]; ok {
			continue
		}
		if len(t.path) != 2 || t.path[0] != "providers" {
			continue
		}
		ft, ok := f.tables[pathKey(t.path)]
		if !ok || ft.array {
			return fmt.Errorf("cannot remove [%s]: it is not a plain table", strings.Join(t.path, "."))
		}
		s.dropSection(ft)
		dropped = append(dropped, t.path)
	}

	for _, kv := range was.kvsInOrder() {
		if _, ok := want.kvs[pathKey(kv.path)]; ok || underAny(kv.path, dropped) {
			continue
		}
		fkv, ok := f.kvs[pathKey(kv.path)]
		if !ok {
			return fmt.Errorf("cannot remove %s: it is not a plain key", strings.Join(kv.path, "."))
		}
		s.add(fkv.start, fkv.end, "")
	}

	inserts := map[string][]string{}
	var insertOrder [][]string
	for _, kv := range want.kvsInOrder() {
		if old, ok := was.kvs[pathKey(kv.path)]; ok && old.value() == kv.value() {
			continue
		}
		if fkv, ok := f.kvs[pathKey(kv.path)]; ok {
			if fkv.valStart >= 0 {
				s.add(fkv.valStart, fkv.valEnd, kv.value())
			} else {
				s.add(fkv.start, fkv.end, indentOf(fkv.line)+fkv.keyText+" = "+kv.value()+s.eol)
			}
			continue
		}
		table := kv.path[:len(kv.path)-1]
		if _, ok := inserts[pathKey(table)]; !ok {
			insertOrder = append(insertOrder, table)
		}
		inserts[pathKey(table)] = append(inserts[pathKey(table)], kv.keyText+" = "+kv.value())
	}
	for _, table := range insertOrder {
		if err := s.insertInto(table, want, inserts[pathKey(table)]); err != nil {
			return err
		}
	}

	// An empty table that is new — a provider added with no settings yet —
	// still needs its header to exist in the map.
	for _, t := range want.tableOrder {
		k := pathKey(t.path)
		if len(t.kvs) > 0 || want.hasSubtable(t.path) {
			continue
		}
		if _, ok := was.tables[k]; ok {
			continue
		}
		if _, ok := f.tables[k]; ok {
			continue
		}
		if s.pendingTable(t.path) == nil {
			s.newTables = append(s.newTables, &newTable{path: t.path, header: t.header})
		}
	}
	return nil
}

// insertInto adds new key lines to table, after its last key, or as a new
// table at the end of the file when the file does not have it yet.
func (s *splicer) insertInto(table []string, want *tomlIndex, lines []string) error {
	f := s.file
	if len(table) == 0 {
		var b strings.Builder
		switch {
		case len(f.rootKVs) > 0:
			last := f.rootKVs[len(f.rootKVs)-1]
			pos := s.lineBreakAfter(last.end, &b)
			for _, l := range lines {
				b.WriteString(indentOf(last.line) + l + s.eol)
			}
			s.add(pos, pos, b.String())
		case f.firstHeader >= 0:
			// Above the first table, but below a comment block that sits
			// directly on it: that comment describes the table.
			pos := f.firstHeader
			for pos > 0 {
				prev := lineStart(f.data, pos-1)
				if !strings.HasPrefix(strings.TrimSpace(string(f.data[prev:pos])), "#") {
					break
				}
				pos = prev
			}
			for _, l := range lines {
				b.WriteString(l + s.eol)
			}
			b.WriteString(s.eol)
			s.add(pos, pos, b.String())
		default:
			pos := s.lineBreakAfter(len(f.data), &b)
			for _, l := range lines {
				b.WriteString(l + s.eol)
			}
			s.add(pos, pos, b.String())
		}
		return nil
	}

	if ft, ok := f.tables[pathKey(table)]; ok {
		if ft.array {
			return fmt.Errorf("cannot edit [[%s]]: it is an array of tables", strings.Join(table, "."))
		}
		indent := ""
		if n := len(ft.kvs); n > 0 {
			indent = indentOf(ft.kvs[n-1].line)
		}
		var b strings.Builder
		pos := s.lineBreakAfter(ft.lastEnd, &b)
		for _, l := range lines {
			b.WriteString(indent + l + s.eol)
		}
		s.add(pos, pos, b.String())
		return nil
	}

	wt, ok := want.tables[pathKey(table)]
	if !ok {
		return fmt.Errorf("no table for %s", strings.Join(table, "."))
	}
	nt := s.pendingTable(table)
	if nt == nil {
		nt = &newTable{path: table, header: wt.header}
		s.newTables = append(s.newTables, nt)
	}
	nt.lines = append(nt.lines, lines...)
	return nil
}

func (s *splicer) pendingTable(path []string) *newTable {
	for _, t := range s.newTables {
		if pathKey(t.path) == pathKey(path) {
			return t
		}
	}
	return nil
}

// lineBreakAfter returns pos, first writing a line break to b when pos is
// the end of a file whose last line has none.
func (s *splicer) lineBreakAfter(pos int, b *strings.Builder) int {
	if pos == len(s.file.data) && pos > 0 && s.file.data[pos-1] != '\n' {
		b.WriteString(s.eol)
	}
	return pos
}

// dropSection deletes a table's header and keys, with the blank lines that
// follow. Comments after its last key are kept: they usually introduce the
// next table.
func (s *splicer) dropSection(t *tomlTable) {
	data := s.file.data
	start, end := t.start, t.lastEnd
	for end < len(data) {
		next := lineEnd(data, end)
		if strings.TrimSpace(string(data[end:next])) != "" {
			break
		}
		end = next
	}
	if end == len(data) {
		// Last in the file: take the blank lines that separated it too.
		for start > 0 {
			prev := lineStart(data, start-1)
			if strings.TrimSpace(string(data[prev:start])) != "" {
				break
			}
			start = prev
		}
	}
	s.add(start, end, "")
}

// apply returns the file with every edit made and new tables appended.
func (s *splicer) apply() ([]byte, error) {
	data := s.file.data
	if len(s.newTables) > 0 {
		var b strings.Builder
		s.lineBreakAfter(len(data), &b)
		for i, t := range s.newTables {
			if len(data) > 0 || i > 0 {
				b.WriteString(s.eol)
			}
			b.WriteString(t.header + s.eol)
			for _, l := range t.lines {
				b.WriteString(l + s.eol)
			}
		}
		s.add(len(data), len(data), b.String())
	}

	edits := s.edits
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		// Insertions before a deletion at the same offset, each group in
		// the order planned.
		ii, ji := edits[i].start == edits[i].end, edits[j].start == edits[j].end
		if ii != ji {
			return ii
		}
		return edits[i].seq < edits[j].seq
	})
	var out bytes.Buffer
	pos := 0
	for _, e := range edits {
		if e.start < pos {
			return nil, errors.New("overlapping edits")
		}
		out.Write(data[pos:e.start])
		out.WriteString(e.text)
		pos = e.end
	}
	out.Write(data[pos:])
	return out.Bytes(), nil
}

func (idx *tomlIndex) kvsInOrder() []*tomlKV {
	out := append([]*tomlKV{}, idx.rootKVs...)
	for _, t := range idx.tableOrder {
		out = append(out, t.kvs...)
	}
	return out
}

func (idx *tomlIndex) hasSubtable(path []string) bool {
	for _, t := range idx.tableOrder {
		if len(t.path) > len(path) && pathKey(t.path[:len(path)]) == pathKey(path) {
			return true
		}
	}
	return false
}

// value is the value as written, for a key from a canonical encoding.
// go-toml writes most strings as 'literal' strings; hand-written configs and
// the documentation use "basic" ones, so a literal is respelled to match.
func (kv *tomlKV) value() string {
	if kv.valStart < 0 {
		return kv.line
	}
	v := kv.line[kv.valStart-kv.start : kv.valEnd-kv.start]
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' && !strings.HasPrefix(v, "'''") {
		inner := v[1 : len(v)-1]
		inner = strings.ReplaceAll(inner, `\`, `\\`)
		inner = strings.ReplaceAll(inner, `"`, `\"`)
		return `"` + inner + `"`
	}
	return v
}

func underAny(path []string, tables [][]string) bool {
	for _, t := range tables {
		if len(path) > len(t) && pathKey(path[:len(t)]) == pathKey(t) {
			return true
		}
	}
	return false
}

func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
