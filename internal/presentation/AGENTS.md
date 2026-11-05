# presentation

Strict decoder for SPEC-0003 §3 semantic presentation snapshots (`schema.go`). SDK-free, stdlib-only, no I/O, no state; renderers, resolvers and lifecycle tracking consume the decoded values.

## Contract

- `Decode([]byte) (Snapshot, error)` accepts a snapshot object or any details object carrying `workbenchPresentation` (SPEC-0003 §2 default `detailsPath`); sibling producer fields are allowed there, unknown fields inside the snapshot are not. Errors match `ErrInvalid` and name the field path only.
- Producers own the snapshot; the decoder never fills gaps. Absent `complete`, `effects`, `target`, `outcome`, `continuation`, `provenance` and `parentCallId` stay nil/empty, an explicit numeric zero stays zero, and a settled snapshot without a valid `outcome` is rejected rather than read as success. `preparing`/`running` must not carry `outcome`.
- Bounds: `MaxPayloadBytes` (checked before any parse), `MaxBlocks`, `MaxMetrics`, `MaxTableRows`, `MaxDepth`. Depth counts the decoded snapshot as 1 and only `children` retained snapshots nest. SPEC-0003 allows lowering limits per registration; never raise them here.
- Closed unions: `Phase`, `Outcome`, `Effects`, `TargetKind`, `MetricUnit` (plus three-uppercase-letter currency), `BlockKind`, `TextRole`, `Severity`, `DiffOperation`, `HunkLineKind`, `CellKind`. `Block` holds exactly one payload pointer matching `Kind`; `Cell` holds `Text` for text/label/path/uri and `Number` for count/bytes/duration.
- Wire shapes the spec leaves open, as decoded:
  - diff — `{operation: create|update|delete|move|copy|other, before?, after?, hunks?}`; `FileIdentity{path, revision?}`; hunk `{oldStart, oldLines, newStart, newLines, lines:[{kind: context|add|remove, text}]}`. At least one of before/after.
  - imageRef — `{resource?, blockId?, mediaType?, alt?}`, at least one reference. Bytes never travel here: `data`/`base64` are unknown fields.
  - table — `{columns:[{id, label, cell}], rows:[[{kind, value}]]}`; a cell's kind must equal its column's declared `cell`.
  - children — `{childCallIds, retained?:[{callId, snapshot}]}`; every retained `callId` must appear in `childCallIds`.
  - continuation — `{explanation, uri?, cursor?}` (explanation required); provenance — `{revision?, source?:{uri, line?, endLine?}, adapter?, view?}`.
- Text permits newline and tab only; every other control character (raw or escaped, ESC/CSI included) is rejected. Error messages never echo offending text.

## Traps

- Don't decode with a bare `json.Unmarshal`: unknown members, duplicate keys and nulls must stay errors, so `checkJSONValue` walks the payload before the `DisallowUnknownFields` decoder. The walker's type mirror is `wire*`; change a field in one place and the other must follow.
- Don't turn the `wire*` structs into the public surface. `Snapshot`, `Block`, `Cell` and friends are what the renderer and lifecycle units exchange; `wireSnapshot` is decode-only.
- Don't add a lenient fallback, default outcome or sanitizer here. Rejection is the contract; renderers still strip terminal sequences before output (SPEC-0003 §5).
- Don't model ordering, monotonicity, host precedence or conflicting-final diagnostics in this package (SPEC-0003 §4). Those belong to the lifecycle consumer of these types.

## Cross-updates

- Change an enum value, unit, cell kind, block shape or bound → update SPEC-0003 §3 and every consumer switch in the same change.
- Adding a block kind means `BlockKind` const, `wireBlock` fields, `exclusive` allowance, `Block` payload plus `encodedBlock`/`MarshalJSON`, and the decoder tests.

Sources: `.project/specs/0003-tool-presentation.md` §3–§5.
