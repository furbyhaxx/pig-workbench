# syntax

Tree-sitter structural facts for reads and hashline block selectors (SPEC-0005 §5, SPEC-0006 §4).
- `Service.Analyze` returns source-line coordinates, exact block spans and summary omissions; callers must refuse ambiguous blocks.
- Columns are 1-based byte columns. Summary declarations retain original source text and starting lines.
- The cache is revision/content/grammar keyed and bounded; callers may mutate returned values.
- Parser failures require a text fallback; lexical guesses never authorize edits. Markdown section spans come from heading nodes, not heading-like fenced text.
