// Package jsconfig answers one question about a JavaScript source: where
// does an extra array element belong, or why does it belong nowhere. It
// holds no dharness spelling — no marker text, no file names, no framework
// names — and it never writes.
package jsconfig

import (
	"bytes"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Anchor is a place in src, in byte offsets into the exact bytes Analyze
// was given. Every field is an offset or a literal read out of the source,
// so a caller splices with slice arithmetic and nothing else.
type Anchor struct {
	// ImportAt is where an import statement may be inserted: the start of
	// the line after the last top-level import declaration, or the start of
	// the file (after a BOM and after a "use strict"-style prologue) when
	// there is none.
	ImportAt int

	// LayerAt is where the array element belongs — always the array's
	// first element, moved back over any contiguous comment run
	// immediately preceding it.
	LayerAt int

	// Indent is the leading whitespace of the anchor line, copied verbatim,
	// so the inserted element sits with its neighbours.
	Indent string

	// LineEnding is "\r\n" or "\n", read off the anchor line rather than
	// chosen. A file that mixes them keeps whatever that one line used.
	LineEnding string
}

// jsLanguage is the tree-sitter grammar every Analyze call parses against —
// a package-level value, not a per-call allocation, since the grammar table
// is immutable and gotreesitter's own registry already caches the decode.
var jsLanguage = grammars.JavascriptLanguage()

// Analyze reports where dharness's layer belongs in src, or why it belongs
// nowhere. ok == false always carries a non-empty why; ok == true always
// carries a complete Anchor. Never both, never neither.
//
// It takes the source and nothing else. There is no framework parameter:
// dharness's layer is always the array's first element, so the position
// needs no knowledge of what the project imported.
func Analyze(src []byte) (a Anchor, why string, ok bool) {
	parser := gotreesitter.NewParser(jsLanguage)
	tree, err := parser.Parse(src)
	if err != nil {
		return Anchor{}, "source does not parse: " + err.Error(), false
	}

	root := tree.RootNode()
	value, why, ok := defaultExportValue(root)
	if !ok {
		return Anchor{}, why, false
	}

	// The ERROR check is unconditional and comes first: an ERROR node
	// anywhere in the default export refuses regardless of which other
	// condition below would otherwise be satisfied (spec's "an ERROR node
	// covering the default export refuses the whole file"). HasError is a
	// propagated flag, so checking it here already covers an ERROR nested
	// inside a recognised defineConfig(...) call's array argument — the
	// spec's narrower three-conditions scenario.
	if value.HasError() {
		return Anchor{}, "the default export's parse contains an ERROR node", false
	}

	array, why, ok := exportedArray(value, root, src)
	if !ok {
		return Anchor{}, why, false
	}

	layerAt, indent, lineEnding := positionRule(array, src)
	return Anchor{
		ImportAt:   importAt(root, src),
		LayerAt:    layerAt,
		Indent:     indent,
		LineEnding: lineEnding,
	}, "", true
}

// Splice returns src with region inserted at at. It is separate from
// Analyze because it is the whole destructive operation and it is worth
// being able to state, in one line, that it is an insert: the result is
// src[:at] + region + src[at:] and nothing else.
func Splice(src []byte, at int, region string) []byte {
	out := make([]byte, 0, len(src)+len(region))
	out = append(out, src[:at]...)
	out = append(out, region...)
	out = append(out, src[at:]...)
	return out
}

// defaultExportValue finds the module's "export default" statement among
// root's top-level children and returns the value it exports — the array
// literal, the call expression, or whatever else follows "export default".
// Only an export_statement with a populated "value" field is a default
// export; "export const"/"export function" etc. set a "declaration" field
// instead and are not this shape.
func defaultExportValue(root *gotreesitter.Node) (*gotreesitter.Node, string, bool) {
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Type(jsLanguage) != "export_statement" {
			continue
		}
		if value := child.ChildByFieldName("value", jsLanguage); value != nil {
			return value, "", true
		}
	}
	return nil, "no default export found", false
}

// exportedArray dispatches on the default export's shape: a plain array
// literal splices directly, a recognised defineConfig(...) call splices its
// array argument, and anything else delegates — a lookalike is not the
// documented shape.
func exportedArray(value, root *gotreesitter.Node, src []byte) (*gotreesitter.Node, string, bool) {
	switch value.Type(jsLanguage) {
	case "array":
		return value, "", true
	case "call_expression":
		return defineConfigArray(value, root, src)
	default:
		return nil, "default export is not an array literal or a recognised defineConfig(...) call", false
	}
}

// defineConfigArray recognises exactly the documented shape: the callee is
// a bare identifier, that identifier is bound by a top-level import whose
// specifier is exactly "eslint/config", and the call's first argument is an
// array literal. All three conditions must hold; any other call expression
// — a member expression like tseslint.config, an identifier imported from
// anywhere else, a locally-defined helper of the same name — delegates.
// Recognition is by import specifier, never by the identifier's spelling.
//
// call is never nil-fielded here: Analyze already rejected value.HasError()
// before dispatching, and a call_expression's "function" and "arguments"
// fields are grammar-mandatory on any error-free parse of one, so neither
// ChildByFieldName lookup below can return nil.
func defineConfigArray(call, root *gotreesitter.Node, src []byte) (*gotreesitter.Node, string, bool) {
	callee := call.ChildByFieldName("function", jsLanguage)
	if callee.Type(jsLanguage) != "identifier" {
		return nil, "default export's callee is not a plain imported identifier", false
	}

	specifier, imported := importTable(root, src)[callee.Text(src)]
	if !imported || specifier != "eslint/config" {
		return nil, "default export's callee is not imported from \"eslint/config\"", false
	}

	args := call.ChildByFieldName("arguments", jsLanguage)
	if args.NamedChildCount() == 0 {
		return nil, "defineConfig(...) call has no array argument", false
	}
	first := args.NamedChild(0)
	if first.Type(jsLanguage) != "array" {
		return nil, "defineConfig(...)'s first argument is not an array literal", false
	}
	return first, "", true
}

// importTable walks the top-level import declarations once and builds
// binding -> module specifier, for exactly one question: whether a callee
// identifier is bound by an import from "eslint/config". Both default and
// named imports are covered, aliased or not; a namespace import
// (import * as ns) is never a bare-identifier callee, so it is not
// represented here. A side-effect-only import (import "polyfill";) carries
// no clause at all and contributes no binding, but scanning continues past
// it to whatever import follows.
func importTable(root *gotreesitter.Node, src []byte) map[string]string {
	table := map[string]string{}
	for i := 0; i < root.NamedChildCount(); i++ {
		stmt := root.NamedChild(i)
		if stmt.Type(jsLanguage) != "import_statement" {
			continue
		}
		// The grammar makes "source" mandatory on any node it types
		// import_statement; a from-clause too broken to parse becomes an
		// ERROR node instead, filtered out by the type check above.
		specifier := stringValue(stmt.ChildByFieldName("source", jsLanguage), src)

		// import_clause, when present, is always the statement's first
		// named child — it precedes "from" and the source string in every
		// shape the grammar produces. A side-effect-only import has none,
		// but "source" is grammar-mandatory, so NamedChildCount() is never
		// 0 here and NamedChild(0) is always safe to read.
		var clause *gotreesitter.Node
		if c := stmt.NamedChild(0); c.Type(jsLanguage) == "import_clause" {
			clause = c
		}
		if clause == nil {
			continue
		}

		for j := 0; j < clause.NamedChildCount(); j++ {
			part := clause.NamedChild(j)
			switch part.Type(jsLanguage) {
			case "identifier":
				table[part.Text(src)] = specifier
			case "named_imports":
				// Every named child of named_imports is an import_specifier
				// — the grammar admits nothing else there — so each one is
				// processed unconditionally rather than filtered by type.
				for k := 0; k < part.NamedChildCount(); k++ {
					spec := part.NamedChild(k)
					binding := spec.ChildByFieldName("alias", jsLanguage)
					if binding == nil {
						binding = spec.ChildByFieldName("name", jsLanguage)
					}
					if binding != nil {
						table[binding.Text(src)] = specifier
					}
				}
			}
		}
	}
	return table
}

// stringValue reads a string node's content, unquoted, via its
// string_fragment child.
func stringValue(str *gotreesitter.Node, src []byte) string {
	for i := 0; i < str.NamedChildCount(); i++ {
		if c := str.NamedChild(i); c.Type(jsLanguage) == "string_fragment" {
			return c.Text(src)
		}
	}
	return ""
}

// positionRule locates where the layer belongs inside array, per the
// position rule: the start of the line of the array's first non-comment
// named child, then moved back over the contiguous comment run immediately
// preceding it. An empty array (or one holding only comments) has no
// element to anchor on, so the position is just past "[".
func positionRule(array *gotreesitter.Node, src []byte) (layerAt int, indent, lineEnding string) {
	firstElement := -1
	for i := 0; i < array.NamedChildCount(); i++ {
		if array.NamedChild(i).Type(jsLanguage) != "comment" {
			firstElement = i
			break
		}
	}

	if firstElement == -1 {
		at := int(array.StartByte()) + 1 // "[" is always exactly one byte
		return at, indentOfLine(src, at), lineEndingAt(src, at)
	}

	anchor := array.NamedChild(firstElement)
	for i := firstElement - 1; i >= 0; i-- {
		comment := array.NamedChild(i)
		if comment.Type(jsLanguage) != "comment" {
			break
		}
		if blankLineBetween(src, comment.EndByte(), anchor.StartByte()) {
			break
		}
		anchor = comment
	}

	at := startOfLine(src, int(anchor.StartByte()))
	return at, indentOfLine(src, at), lineEndingAt(src, at)
}

// importAt locates where a new top-level import statement may be inserted:
// the start of the line after the last existing import, or — when there is
// none — the start of the file, after any BOM (root.StartByte() already
// excludes it) and after any directive-prologue statements such as
// "use strict";.
func importAt(root *gotreesitter.Node, src []byte) int {
	var lastImport *gotreesitter.Node
	for i := 0; i < root.NamedChildCount(); i++ {
		if c := root.NamedChild(i); c.Type(jsLanguage) == "import_statement" {
			lastImport = c
		}
	}
	if lastImport != nil {
		return startOfNextLine(src, int(lastImport.EndByte()))
	}

	pos := int(root.StartByte())
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if !isDirectivePrologue(child) {
			break
		}
		pos = startOfNextLine(src, int(child.EndByte()))
	}
	return pos
}

// isDirectivePrologue reports whether n is a directive like "use strict";
// — an expression_statement whose sole child is a string literal.
func isDirectivePrologue(n *gotreesitter.Node) bool {
	if n.Type(jsLanguage) != "expression_statement" {
		return false
	}
	if n.NamedChildCount() != 1 {
		return false
	}
	return n.NamedChild(0).Type(jsLanguage) == "string"
}

// startOfLine returns the byte offset of the start of the line containing
// pos: the byte immediately after the nearest preceding "\n", or 0 when pos
// is on the file's first line.
func startOfLine(src []byte, pos int) int {
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

// startOfNextLine returns the byte offset immediately after the next "\n"
// at or after pos, or len(src) when pos's line is the file's last.
func startOfNextLine(src []byte, pos int) int {
	i := bytes.IndexByte(src[pos:], '\n')
	if i == -1 {
		return len(src)
	}
	return pos + i + 1
}

// indentOfLine returns the leading spaces and tabs starting at lineStart,
// copied verbatim.
func indentOfLine(src []byte, lineStart int) string {
	i := lineStart
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return string(src[lineStart:i])
}

// lineEndingAt reports "\r\n" or "\n", read off the next line break at or
// after from — the anchor line's own ending, not a chosen convention.
// Defaults to "\n" when from's line is the file's last and carries no
// trailing newline at all.
func lineEndingAt(src []byte, from int) string {
	i := bytes.IndexByte(src[from:], '\n')
	if i == -1 {
		return "\n"
	}
	nl := from + i
	if nl > 0 && src[nl-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// blankLineBetween reports whether the bytes strictly between from and to
// contain a blank line — two or more "\n" bytes, meaning an empty line
// separates them. This is the comment-run's contiguity boundary: a comment
// separated from the next element by a blank line is a section header, not
// a description of that element.
func blankLineBetween(src []byte, from, to uint32) bool {
	return bytes.Count(src[from:to], []byte("\n")) >= 2
}
