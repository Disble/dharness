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

// Module is the module system a config file is written in. It is read off
// the file rather than chosen: ESLint accepts flat config in either, and the
// two frameworks dharness has presets for disagree — Next.js documents an
// eslint.config.mjs using `import`/`export default`, and `npx expo lint`
// generates an eslint.config.js using `require`/`module.exports`.
//
// A caller needs it because the declaration it splices in has to match:
// an `import` statement in a CommonJS file is a SyntaxError, and so is
// `require` reaching for a binding in a file Node has decided is ESM.
type Module int

const (
	// ESM is `import x from "y"` and `export default [...]`.
	ESM Module = iota

	// CommonJS is `const x = require("y")` and `module.exports = [...]`.
	CommonJS
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
	value, _, why, ok := exportedValue(root, src)
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

// ModuleOf reports which module system src is written in.
//
// It is separate from Analyze because it answers a different question with a
// different tolerance. Analyze asks "where does the layer go, or why
// nowhere", and refuses everything it does not recognise. This asks "which
// dialect do I write a declaration in", and always answers — a caller
// replacing a marked region it wrote earlier needs the dialect without
// re-establishing that the whole file is still spliceable.
//
// The export form decides, because that is the definitive marker: a file
// that says "module.exports" is CommonJS whatever else it contains. With no
// recognised export, the import form decides. With neither — an empty file,
// or one this package does not recognise at all — the answer is ESM, which
// is both ESLint's own documented default and what dharness writes when it
// creates the file itself.
func ModuleOf(src []byte) Module {
	parser := gotreesitter.NewParser(jsLanguage)
	tree, err := parser.Parse(src)
	if err != nil {
		return ESM
	}

	root := tree.RootNode()
	if _, module, _, ok := exportedValue(root, src); ok {
		return module
	}

	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Type(jsLanguage) == "import_statement" {
			return ESM
		}
		if isRequireDeclaration(child, src) {
			return CommonJS
		}
	}
	return ESM
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

// exportedValue finds the value this config exports and the module system it
// exported it with — the array literal, the call expression, or whatever else
// the export names.
//
// Two shapes are recognised, because ESLint accepts both and the frameworks
// generate both. "export default <value>" is ESM: only an export_statement
// with a populated "value" field is a default export, since
// "export const"/"export function" set a "declaration" field instead.
// "module.exports = <value>" is CommonJS, which is what `npx expo lint`
// writes.
//
// The scan is single-pass and takes the first of either, so a file holding
// both is read as whatever it states first rather than being refused —
// ESLint would load one of them and this reports on the same one.
func exportedValue(root *gotreesitter.Node, src []byte) (*gotreesitter.Node, Module, string, bool) {
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		switch child.Type(jsLanguage) {
		case "export_statement":
			if value := child.ChildByFieldName("value", jsLanguage); value != nil {
				return boundValue(value, root, src), ESM, "", true
			}
		case "expression_statement":
			if value, ok := moduleExportsValue(child, src); ok {
				return value, CommonJS, "", true
			}
		}
	}
	return nil, ESM, "no default export or module.exports assignment found", false
}

// boundValue follows a default export that names a variable back to the
// value that variable was declared with, or returns value unchanged when
// there is nothing to follow.
//
// It exists because `create-next-app` binds before it exports:
//
//	const eslintConfig = defineConfig([...]);
//	export default eslintConfig;
//
// Measured against Next.js 16.3.1, that is what the framework's own
// scaffolder emits today. Without this hop the export is an identifier, and
// every project it creates is refused for a shape that is fully recognised
// one line up.
//
// Exactly one hop, and only into a top-level declaration. Following a chain
// would mean answering for reassignment between the declarations, which this
// package does not track; an identifier that resolves to nothing is returned
// as it came, so the caller refuses it with the sentence it already has.
func boundValue(value, root *gotreesitter.Node, src []byte) *gotreesitter.Node {
	if value.Type(jsLanguage) != "identifier" {
		return value
	}

	name := value.Text(src)
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Type(jsLanguage) != "lexical_declaration" && child.Type(jsLanguage) != "variable_declaration" {
			continue
		}
		for j := 0; j < child.NamedChildCount(); j++ {
			declarator := child.NamedChild(j)
			if declarator.Type(jsLanguage) != "variable_declarator" {
				continue
			}
			bound := declarator.ChildByFieldName("name", jsLanguage)
			initial := declarator.ChildByFieldName("value", jsLanguage)
			if bound == nil || initial == nil || bound.Text(src) != name {
				continue
			}
			return initial
		}
	}
	return value
}

// moduleExportsValue reports the right-hand side of a top-level
// "module.exports = <value>" statement.
//
// The left-hand side is matched on its source text rather than by walking
// object and property identifiers separately: "module.exports" is the whole
// name, and a member expression that merely ends in ".exports" — a project's
// own "config.exports" — is not it. "exports = [...]" alone is deliberately
// not recognised: it does not replace the module's exports in Node, so
// treating it as the config would splice into a value ESLint never reads.
func moduleExportsValue(stmt *gotreesitter.Node, src []byte) (*gotreesitter.Node, bool) {
	if stmt.NamedChildCount() == 0 {
		return nil, false
	}
	assignment := stmt.NamedChild(0)
	if assignment.Type(jsLanguage) != "assignment_expression" {
		return nil, false
	}

	left := assignment.ChildByFieldName("left", jsLanguage)
	right := assignment.ChildByFieldName("right", jsLanguage)
	if left == nil || right == nil {
		return nil, false
	}
	if left.Type(jsLanguage) != "member_expression" || left.Text(src) != "module.exports" {
		return nil, false
	}
	return right, true
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

	specifier, bound := bindingTable(root, src)[callee.Text(src)]
	if !bound || specifier != "eslint/config" {
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

// bindingTable walks the top-level declarations once and builds
// binding -> module specifier, for exactly one question: whether a callee
// identifier is bound to "eslint/config".
//
// Both module systems are walked, because the callee it has to recognise is
// written either way: `import { defineConfig } from "eslint/config"` and
// `const { defineConfig } = require("eslint/config")` are the same statement
// in the two dialects, and Expo's generated config uses the second.
//
// For imports, both default and named forms are covered, aliased or not; a
// namespace import (import * as ns) is never a bare-identifier callee, so it
// is not represented here. A side-effect-only import (import "polyfill";)
// carries no clause at all and contributes no binding, but scanning
// continues past it to whatever import follows.
func bindingTable(root *gotreesitter.Node, src []byte) map[string]string {
	table := map[string]string{}
	for i := 0; i < root.NamedChildCount(); i++ {
		stmt := root.NamedChild(i)
		if isRequireDeclaration(stmt, src) {
			addRequireBindings(table, stmt, src)
			continue
		}
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

// isRequireDeclaration reports whether stmt is a top-level declaration that
// binds at least one name to a require() call — the CommonJS equivalent of
// an import statement, and the shape importAt anchors after.
func isRequireDeclaration(stmt *gotreesitter.Node, src []byte) bool {
	for _, declarator := range requireDeclarators(stmt, src) {
		if declarator != nil {
			return true
		}
	}
	return false
}

// requireDeclarators returns stmt's variable declarators whose value is a
// require() call, or nil when stmt is not a declaration at all. A single
// declaration can bind several (`const a = require("x"), b = require("y")`),
// and one that mixes a require with an ordinary initialiser contributes only
// the require.
func requireDeclarators(stmt *gotreesitter.Node, src []byte) []*gotreesitter.Node {
	switch stmt.Type(jsLanguage) {
	case "lexical_declaration", "variable_declaration":
	default:
		return nil
	}

	var declarators []*gotreesitter.Node
	for i := 0; i < stmt.NamedChildCount(); i++ {
		declarator := stmt.NamedChild(i)
		if declarator.Type(jsLanguage) != "variable_declarator" {
			continue
		}
		value := declarator.ChildByFieldName("value", jsLanguage)
		if value == nil {
			continue
		}
		if _, ok := requireSpecifier(value, src); ok {
			declarators = append(declarators, declarator)
		}
	}
	return declarators
}

// requireSpecifier reads the module specifier out of a `require("x")` call.
// The callee must be the bare identifier "require" and the call must carry
// exactly one string argument; a member expression like `createRequire(...)`
// or a computed specifier is not this shape and is not recognised, for the
// same reason defineConfigArray refuses a lookalike.
func requireSpecifier(call *gotreesitter.Node, src []byte) (string, bool) {
	if call.Type(jsLanguage) != "call_expression" {
		return "", false
	}
	callee := call.ChildByFieldName("function", jsLanguage)
	if callee == nil || callee.Type(jsLanguage) != "identifier" || callee.Text(src) != "require" {
		return "", false
	}
	args := call.ChildByFieldName("arguments", jsLanguage)
	if args == nil || args.NamedChildCount() != 1 {
		return "", false
	}
	first := args.NamedChild(0)
	if first.Type(jsLanguage) != "string" {
		return "", false
	}
	return stringValue(first, src), true
}

// addRequireBindings records every name stmt binds to a require() specifier:
// a plain `const x = require(...)`, and each name of a destructuring
// `const { a, b: c } = require(...)`. A destructured name is what makes
// Expo's `const { defineConfig } = require("eslint/config")` recognisable as
// the same statement as its ESM spelling.
func addRequireBindings(table map[string]string, stmt *gotreesitter.Node, src []byte) {
	for _, declarator := range requireDeclarators(stmt, src) {
		specifier, ok := requireSpecifier(declarator.ChildByFieldName("value", jsLanguage), src)
		if !ok {
			continue
		}
		name := declarator.ChildByFieldName("name", jsLanguage)
		if name == nil {
			continue
		}
		switch name.Type(jsLanguage) {
		case "identifier":
			table[name.Text(src)] = specifier
		case "object_pattern":
			for i := 0; i < name.NamedChildCount(); i++ {
				part := name.NamedChild(i)
				switch part.Type(jsLanguage) {
				case "shorthand_property_identifier_pattern":
					table[part.Text(src)] = specifier
				case "pair_pattern":
					if alias := part.ChildByFieldName("value", jsLanguage); alias != nil && alias.Type(jsLanguage) == "identifier" {
						table[alias.Text(src)] = specifier
					}
				}
			}
		}
	}
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

// importAt locates where a new top-level import may be inserted: the start
// of the line after the last existing one, or — when there is none — the
// start of the file, after any BOM (root.StartByte() already excludes it)
// and after any directive-prologue statements such as "use strict";.
//
// "Existing one" spans both dialects: an import_statement, or a declaration
// binding a require() call. A CommonJS config has no import statements at
// all, and anchoring at the top of the file would put the spliced require
// above the "use strict" prologue rather than below it.
func importAt(root *gotreesitter.Node, src []byte) int {
	var last *gotreesitter.Node
	for i := 0; i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c.Type(jsLanguage) == "import_statement" || isRequireDeclaration(c, src) {
			last = c
		}
	}
	if last != nil {
		return startOfNextLine(src, int(last.EndByte()))
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
