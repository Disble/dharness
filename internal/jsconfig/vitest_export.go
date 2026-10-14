package jsconfig

import (
	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// tsLanguage is the grammar AnalyzeExport parses against when its caller
// says the source is TypeScript. Held as a package-level value for the same
// reason jsLanguage is: the grammar table is immutable and gotreesitter's
// registry already caches the decode.
var tsLanguage = grammars.TypescriptLanguage()

// ExportSpan is where a module's configuration export lives, in byte offsets
// into the exact bytes AnalyzeExport was given. Every field is an offset or a
// name read out of the source, so a caller splices with slice arithmetic and
// nothing else.
//
// It is a span, not a judgement: it says where the consumer's own bytes are,
// never what they mean. Splitting the value further, resolving an identifier
// to the declaration that binds it, and merging the caller's configuration
// into the consumer's are all the caller's decisions.
type ExportSpan struct {
	// StatementStart and StatementEnd delimit the whole export statement —
	// "export default <value>;" or "module.exports = <value>;" — including
	// its terminating semicolon when it has one.
	StatementStart int
	StatementEnd   int

	// ValueStart and ValueEnd delimit the exported value exactly as the
	// consumer wrote it: an object literal, a defineConfig(...) call, a
	// function or arrow function, a TypeScript expression such as one ending
	// in "satisfies UserConfig", or the bare identifier of an export that
	// names a binding declared elsewhere.
	//
	// An identifier export is reported as the identifier itself. The
	// initializer that identifier was declared with is deliberately not
	// followed, because a caller rewriting the export owns that hop's
	// consequences and this package resolves no consumer binding for it.
	ValueStart int
	ValueEnd   int

	// ImportAt is where a top-level import may be inserted: the start of the
	// line after the last import or require declaration, or the start of the
	// file (after a BOM and a "use strict"-style prologue) when there is
	// none. It carries the same meaning as Anchor.ImportAt.
	ImportAt int

	// Dialect is the module system the export itself is written in:
	// "export default" is ESM, "module.exports" is CommonJS.
	Dialect Module

	// DefaultFunctionName is the name bound by a default function
	// declaration — "export default function config() {}",
	// "export default async function config() {}",
	// "export default function* config() {}" — because a caller that
	// rewrites that statement has to keep the declaration's local binding
	// alive separately from the export it wraps.
	//
	// It is empty for every other shape: an anonymous function expression
	// has no binding, a function expression's own name is visible only
	// inside itself, and a class declaration's name stays inside the value
	// span for the caller to read there.
	DefaultFunctionName string
}

// AnalyzeExport reports where a module's configuration export lives in src,
// or why no unambiguous one could be found. ok == false always carries a
// non-empty why and the zero span; ok == true always carries a complete span.
// Never both, never neither.
//
// typescript selects the grammar; the caller knows the file extension and
// this package does not. It is a decision rather than a tolerance: the
// TypeScript grammar parses every shape the JavaScript one does, and the
// extra syntax a config legitimately uses — a "satisfies UserConfig"
// clause, a type annotation on the exported function — only parses under it.
//
// The parse never evaluates consumer code. A caller holding the span reads
// the consumer's own bytes out of src and decides what to do with them:
//
//	span, why, ok := jsconfig.AnalyzeExport(src, true)
//	if !ok {
//		return fmt.Errorf("config export: %s", why)
//	}
//	raw := src[span.ValueStart:span.ValueEnd] // what the consumer wrote
//	withImport := jsconfig.Splice(src, span.ImportAt, importLine)
//
// Refusal is for what cannot be bounded: source whose parse contains a
// syntax error, no default export or module.exports at all, or more than one
// of them. An exported value this package does not otherwise recognise is
// still reported when its bytes are unambiguous — bounding the export is the
// question here, not reading the configuration inside it.
func AnalyzeExport(src []byte, typescript bool) (ExportSpan, string, bool) {
	lang := jsLanguage
	if typescript {
		lang = tsLanguage
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		return ExportSpan{}, "source does not parse: " + err.Error(), false
	}

	root := tree.RootNode()
	if root.HasError() {
		return ExportSpan{}, "source contains a syntax error", false
	}

	statement, value, dialect, why, ok := exportStatement(root, src, lang)
	if !ok {
		return ExportSpan{}, why, false
	}

	return ExportSpan{
		StatementStart:      int(statement.StartByte()),
		StatementEnd:        int(statement.EndByte()),
		ValueStart:          int(value.StartByte()),
		ValueEnd:            int(value.EndByte()),
		ImportAt:            exportImportAt(root, src, lang),
		Dialect:             dialect,
		DefaultFunctionName: defaultFunctionName(value, src, lang),
	}, "", true
}

// exportStatement finds the one statement that exports this module's
// configuration, or says why there is not exactly one.
//
// Two shapes are recognised, one per dialect. "export default <value>" is
// ESM: the exported value is the export_statement's "value" field, and a
// function or class declaration arrives in its "declaration" field instead.
// That field is also where a *named* export keeps its declaration, so the
// "default" keyword is required before either field is read — otherwise
// "export const config = {...}" would read as an export of the declaration.
// "module.exports = <value>" is CommonJS, matched on the whole left-hand
// spelling like the ESLint path matches it: a member expression merely
// ending in ".exports", and a bare "exports = ...", do not replace the
// module's exports in Node.
//
// Exactly one candidate is required. A file holding two — two default
// exports, or a default export and a module.exports — is ambiguous, and a
// file holding none has nothing to splice around.
func exportStatement(root *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (statement, value *gotreesitter.Node, dialect Module, why string, ok bool) {
	candidates := 0
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if exported, childDialect, isExport := exportedIn(child, src, lang); isExport {
			statement, value, dialect = child, exported, childDialect
			candidates++
		}
	}

	switch {
	case candidates == 0:
		return nil, nil, ESM, "no default export or module.exports assignment found", false
	case candidates > 1:
		return nil, nil, ESM, "more than one default export or module.exports assignment found", false
	}
	return statement, value, dialect, "", true
}

// exportedIn reports the value a single top-level statement exports, and
// which dialect it is written in, or that the statement exports nothing this
// package can bound.
//
// It is a helper rather than a switch inside exportStatement's scan because
// the scan has to keep looking past a non-export: a config may open with an
// ordinary statement or a named export before its one export. Answering one
// statement at a time leaves the scan with an unconditional advance and no
// continue/break to be confused with a switch's own break.
func exportedIn(stmt *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (*gotreesitter.Node, Module, bool) {
	switch stmt.Type(lang) {
	case "export_statement":
		if !hasDefaultKeyword(stmt, lang) {
			return nil, ESM, false
		}
		// The keyword settles first, because "export const config = {...}"
		// keeps its declaration in the same field that "export default
		// function config() {}" uses. Once the child carrying it is the
		// grammar's own "default" token, the alternative it belongs to is
		// "default" followed by exactly one of these two fields, so on a
		// parse without an error — which AnalyzeExport has already refused
		// otherwise — one of the two lookups below returns the export.
		exported := stmt.ChildByFieldName("value", lang)
		if exported == nil {
			exported = stmt.ChildByFieldName("declaration", lang)
		}
		return exported, ESM, true
	case "expression_statement":
		exported, found := moduleExportsRight(stmt, src, lang)
		return exported, CommonJS, found
	default:
		return nil, ESM, false
	}
}

// hasDefaultKeyword reports whether stmt is "export default ..." rather than
// a named export. The keyword is matched on the child's node type, and only
// among the statement's direct children.
//
// Type, not spelling: an anonymous token is typed by its own text — which is
// what the grammar spells the keyword as — while a named child is typed by
// its rule. So this asks "does the statement carry the keyword token", not
// "does some child spell default", and no named child of any other export
// shape can be mistaken for the keyword.
//
// Direct children only, for the same reason: "export { a as default }"
// carries "default" inside its export_clause, and "export * as default from
// ..." inside its namespace_export, so both re-export a binding instead of
// exporting a value this package can bound.
func hasDefaultKeyword(stmt *gotreesitter.Node, lang *gotreesitter.Language) bool {
	for _, child := range stmt.Children() {
		if child.Type(lang) == "default" {
			return true
		}
	}
	return false
}

// moduleExportsRight reports the right-hand side of a top-level
// "module.exports = <value>" statement.
//
// Three of its reads drop a guard the parse has already spent. An
// expression_statement always has its expression as its first named child
// ("expression_statement: _expressions _semicolon", with "_expressions" an
// expression or a non-empty sequence_expression), and an
// assignment_expression always carries both "left" and "right"; the grammar
// admits no error-free tree where either is missing, and AnalyzeExport
// refused an erroring tree before this ran.
//
// The left-hand side is then matched on its text alone. "module.exports" is
// a spelling only a member expression produces, so testing the node type as
// well would restate what the bytes already decide. Every other target
// differs in its bytes and is refused here: a project's own "config.exports",
// "module.exports.default", and a parenthesised "(module.exports)" alike.
func moduleExportsRight(stmt *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (*gotreesitter.Node, bool) {
	assignment := stmt.NamedChild(0)
	if assignment.Type(lang) != "assignment_expression" {
		return nil, false
	}
	if assignment.ChildByFieldName("left", lang).Text(src) != "module.exports" {
		return nil, false
	}
	return assignment.ChildByFieldName("right", lang), true
}

// defaultFunctionName reads the name a default function declaration binds —
// the one shape whose name has to survive a caller replacing the statement.
// A named generator is a declaration of its own type in this grammar, so
// both spellings are read. The anonymous form parses as a function
// expression, which binds nothing outside itself.
func defaultFunctionName(value *gotreesitter.Node, src []byte, lang *gotreesitter.Language) string {
	if t := value.Type(lang); t != "function_declaration" && t != "generator_function_declaration" {
		return ""
	}
	name := value.ChildByFieldName("name", lang)
	if name == nil {
		return ""
	}
	return name.Text(src)
}

// exportImportAt locates where a new top-level import may be inserted: the
// start of the line after the last existing import or require declaration, or
// — when there is none — the start of the file, after any BOM and after any
// directive prologue such as "use strict";.
//
// It is spelled out again here rather than shared with importAt because that
// one is pinned to the JavaScript grammar and is covered by the ESLint path's
// own proven scenarios: AnalyzeExport has to ask the same question of
// whichever grammar the caller named, and threading a language through the
// ESLint helpers would have edited that proven code for no behavioural gain.
// Unlike the ESLint helpers, this one reads no module specifier — only
// whether a binding comes from a require() call, which is all the boundary
// needs.
func exportImportAt(root *gotreesitter.Node, src []byte, lang *gotreesitter.Language) int {
	var last *gotreesitter.Node
	for i := 0; i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c.Type(lang) == "import_statement" || bindsRequire(c, src, lang) {
			last = c
		}
	}
	if last != nil {
		return startOfNextLine(src, int(last.EndByte()))
	}

	pos := int(root.StartByte())
	// Children rather than named children: a program node's children are the
	// statements themselves, so both visits meet the same nodes, and this one
	// needs no index bound whose last out-of-range step no valid input can
	// reach.
	for _, child := range root.Children() {
		if !isDirectivePrologueIn(child, lang) {
			break
		}
		pos = startOfNextLine(src, int(child.EndByte()))
	}
	return pos
}

// bindsRequire reports whether stmt is a top-level declaration binding at
// least one name to a require() call — the CommonJS equivalent of an import
// statement, and the shape exportImportAt anchors after.
//
// Every child is visited rather than the named ones, because every named
// child of a lexical_declaration or variable_declaration is a
// variable_declarator: reading the "value" field off each child meets exactly
// the declarators, and the anonymous kind keyword, separators and semicolon
// between them answer with no field and fall through. The nil test on that
// field is not about the shape: a declarator may carry no initializer at all
// ("let pending;").
func bindsRequire(stmt *gotreesitter.Node, src []byte, lang *gotreesitter.Language) bool {
	switch stmt.Type(lang) {
	case "lexical_declaration", "variable_declaration":
	default:
		return false
	}

	for _, child := range stmt.Children() {
		if value := child.ChildByFieldName("value", lang); value != nil && isRequireCall(value, src, lang) {
			return true
		}
	}
	return false
}

// isRequireCall reports whether call is a require("<one string>") call. The
// callee must be the bare identifier "require"; a member expression or a
// computed specifier is not this shape.
//
// The "function" and "arguments" fields are mandatory in every alternative
// of this grammar's call_expression — "arguments" is a call's argument list
// or a tagged template's template_string — so on the error-free parse
// AnalyzeExport has already established, neither lookup can come back nil.
// The callee is read by its text, which is exact rather than loose: only the
// bare identifier spells "require", since every other callee keeps its
// qualifier or its punctuation inside those bytes.
func isRequireCall(call *gotreesitter.Node, src []byte, lang *gotreesitter.Language) bool {
	if call.Type(lang) != "call_expression" {
		return false
	}
	if call.ChildByFieldName("function", lang).Text(src) != "require" {
		return false
	}
	args := call.ChildByFieldName("arguments", lang)
	if args.NamedChildCount() != 1 {
		return false
	}
	return args.NamedChild(0).Type(lang) == "string"
}

// isDirectivePrologueIn reports whether n is a directive like "use strict";
// — an expression_statement whose sole child is a string literal — read with
// the grammar the caller named.
func isDirectivePrologueIn(n *gotreesitter.Node, lang *gotreesitter.Language) bool {
	if n.Type(lang) != "expression_statement" {
		return false
	}
	if n.NamedChildCount() != 1 {
		return false
	}
	return n.NamedChild(0).Type(lang) == "string"
}
