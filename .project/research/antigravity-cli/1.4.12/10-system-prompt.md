---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-10-system-prompt"
  title: "agy 1.4.12 system prompt"
  description: "Recovered prompt fragments, templates, assembly symbols, formatter types and unresolved selectors"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:57+01:00"
  updated_at: "2026-11-03T11:52:57+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["recover prompt templates and worker prompt", "record assembly evidence and unresolved selectors"]
---

# agy 1.4.12 — system prompt

## Scope and confidence

[OBS] The target is the installed 234,144,144-byte `agy` 1.4.12; the symbol
cache reports 174,452 functions. The extraction reads the executable without
sending a model request. [UNK] No complete default-session system prompt was
captured; section order, dynamic values, selection predicates, and the complete
account/model-specific differences remain unresolved. The texts below must not
be concatenated as though all were sent in one request.

[OBS] [`evidence/system-prompt/extraction-index.txt`](evidence/system-prompt/extraction-index.txt)
records offsets, byte counts, hashes, and the extraction command. The files
linked below contain raw, unmodified UTF-8 bytes; templates include directives
and comments, which are **not** necessarily present in rendered output. The
binary's embedded template records identify filenames, text pointers, and
lengths; see [`template-records.txt`](evidence/system-prompt/template-records.txt).
Raw seed occurrences alone do not establish prompt boundaries.

## Identity: Antigravity coding assistant

[OBS] The 821-byte [`template-identity.txt`](evidence/system-prompt/template-identity.txt)
is embedded as `system_prompts/identity.tmpl` at `0x5e4ce41`; its identity
sentence starts at `0x5e4ce73`. Its conditional `.IsAutonomous` selects
`working for` versus `pair programming with` and adds a no-questions rule in
the autonomous branch. Go-template whitespace control and comments affect the
rendered text. [UNK] The default CLI value of `.IsAutonomous`, the final section
order, and whether this identity is used for any particular model or account
have not been established.

[OBS] A separate 132-byte [`identity-coding.txt`](evidence/system-prompt/identity-coding.txt)
at `0x5e05bc7` contains the same identity sentence without the other template
content. [UNK] It is not proven to be a complete system prompt, an additional
variant, or separately selected for a CLI session.

## Identity: Antigravity assistant

[OBS] The 90-byte [`identity-assistant.txt`](evidence/system-prompt/identity-assistant.txt)
at `0x5dea233` is a standalone embedded string. [UNK] Its selector, surrounding
sections, and whether it reaches a default `agy` request are unresolved.

## Cumulative default: Antigravity Agent

[OBS] The 106-byte [`identity-agent.txt`](evidence/system-prompt/identity-agent.txt)
at `0x5df61fb` is the first element of a five-string slice. [INF] The complete
779-byte newline-joined result of `DefaultSystemPromptForCumulativePrompt`
when its input pointer is nil or the byte at input offset `0xc` is zero is
[`cumulative-antigravity-agent.txt`](evidence/system-prompt/cumulative-antigravity-agent.txt),
reconstructed from its instructions rather than a runtime capture.
The byte ranges and join separator are documented in
[`cumulative-index.txt`](evidence/system-prompt/cumulative-index.txt); the
slice relocations and branch instructions are in
[`cumulative-layout.txt`](evidence/system-prompt/cumulative-layout.txt) and
[`cumulative-disassembly.txt`](evidence/system-prompt/cumulative-disassembly.txt).
[OBS] `(*commandPromptCacheClient).BuildPrompt` calls this routine at
`0x900b67a`; see [`cumulative-caller.txt`](evidence/system-prompt/cumulative-caller.txt).
[UNK] The semantic name of the tested byte and whether this routine supplies
the main conversational CLI prompt for a particular account are unresolved.

## Cumulative default: Cascade

[INF] The same function joins a different five-string slice when its input is
non-nil and the byte at offset `0xc` is nonzero. The statically reconstructed
860-byte result, starting with a Cascade identity and mentioning Windsurf, is
[`cumulative-cascade.txt`](evidence/system-prompt/cumulative-cascade.txt).
[UNK] Its actual runtime activation and relation to the current CLI default
remain unknown.

## Built-in coding worker

[OBS] The `deepcoder.WorkerL0Agent` symbol has a recovered cache address
`0x8050160`. The disassembly at `0x80501fd` loads `0x5e8e54c` with length
`0x6eb`, then `0x5e76a2f` with length `0x57e` and `0x5e57892` with length
`0x3bd`, before a concatenation call; see
[`functions.txt`](evidence/system-prompt/functions.txt). [INF] The resulting
4,134 bytes are reconstructed as [`worker-l0.txt`](evidence/system-prompt/worker-l0.txt).
[OBS] The bytes include workflow, reporting-format and prompt-protection text.
[INF] This is a built-in deepcoder worker prompt, not evidence that the
main CLI conversation receives it. [UNK] The runtime condition that enables
this worker and any additional sections appended downstream remain unknown.

## Other embedded sections

[OBS] These are independently length-delimited files under the embedded
`template_provider/templates/system_prompts/` tree. Their contents and
conditionals are exact in the linked evidence; inclusion and assembly order
are not established.

| section | file offset | raw template |
|---|---:|---|
| artifacts | `0x5fbaffc` | [artifacts](evidence/system-prompt/template-artifacts.txt) |
| communication style | `0x5eff11a` | [communication style](evidence/system-prompt/template-communication-style.txt) |
| conversation transcript | `0x5e956d2` | [conversation transcript](evidence/system-prompt/template-conversation-transcript.txt) |
| ephemeral message | `0x5e2237f` | [ephemeral message](evidence/system-prompt/template-ephemeral-message.txt) |
| batch function call | `0x5e3eae2` | [function call](evidence/system-prompt/template-function-call.txt) |
| guidelines | `0x5e6d319` | [guidelines](evidence/system-prompt/template-guidelines.txt) |
| identity | `0x5e4ce41` | [identity](evidence/system-prompt/template-identity.txt) |
| persistent context | `0x5f1c69b` | [persistent context](evidence/system-prompt/template-persistent-context.txt) |
| plugins | `0x5e608e6` | [plugins](evidence/system-prompt/template-plugins.txt) |
| skills | `0x5e8c9be` | [skills](evidence/system-prompt/template-skills.txt) |
| slash commands | `0x5e3f45c` | [slash commands](evidence/system-prompt/template-slash-commands.txt) |
| async termination helper | `0x5e4a346` | [async termination](evidence/system-prompt/template-async-termination-rule.txt) |
| test funcmap | `0x5e08ba8` | [test funcmap](evidence/system-prompt/template-test-funcmap.txt) |

[OBS] The skills template conditionally renders preloaded skill content,
available skill names, paths and descriptions from `.Data`; the plugins
template renders names, paths, skills and agents from `.Data`. The communication
template tests `.IsAutonomous`, a LaTeX-support setting, and several model enum
values. The batch-function template itself instructs parallel calls for
independent operations. [UNK] None of those observations establishes which
sections or dynamic values a default session actually uses.

## Assembly and tool-call formatting

[OBS] Recovered symbol names include `systemprompt.Construct` (`0x8987100`),
`systemprompt.Build` (`0x89873a0`), `(*PromptBuilder).Build` (`0x89758e0`),
`(*CascadeConversationalMixin).GetSystemPrompt` (`0x89661a0`),
`UpdateGetChatMessageRequestWithToolFormatter` (`0x82a9640`), and Hermes/XML
`FormatSystemPrompt` methods (`0x82a3560`, `0x82a7dc0`); see
[`functions.txt`](evidence/system-prompt/functions.txt). These are recovered
cache addresses. The cache's function spans can begin inside preceding
instructions, as visible in the disassembly; inspect the actual instructions
before attributing a literal to a function.

[OBS] Direct calls in the inspected code connect `GetSystemPrompt` and
`systemprompt.Build` to `PromptBuilder.AddSections` and `PromptBuilder.Build`.
`PromptBuilder.Build` directly calls methods named `includesSection`,
`processContent`, `applyPromptSectionCustomizations`, and
`resolveInlineTemplates`; see [`callgraph.txt`](evidence/system-prompt/callgraph.txt).
[UNK] Call presence does not establish the executed branch or section order.

[OBS] The embedded `ToolFormatterType` descriptor at `0x641a046` supplies
`UNSPECIFIED=0`, `LLAMA_3=1`, `HERMES=2`, `XML=3`, `CHAT_TRANSCRIPT=4`,
`NONE=5`; see [`tool-formatter-enum.txt`](evidence/system-prompt/tool-formatter-enum.txt).

[OBS] `HermesToolFormatter.FormatSystemPrompt` loads a 3,727-byte format string
at `0x5f1f200`: the exact bytes are in
[`formatter-hermes.txt`](evidence/system-prompt/formatter-hermes.txt). Its `%s`
placeholder is inside `<tools>` tags, and it instructs the model to return
JSON objects inside `<tool_call>` tags. [INF] The method builds JSON tool
records, formats the template and concatenates it with an existing string; the
key instructions and ranges are recorded in
[`formatter-disassembly.txt`](evidence/system-prompt/formatter-disassembly.txt).
[INF] The existing string is the incoming system prompt and the formatted block
is appended; selection of Hermes for a specific model has not been observed.
[OBS] The second `write_to_file` example in the literal contains invalid JSON:
it uses a colon after the name value where a comma would be needed. This is
part of the embedded example, not evidence of an actual malformed tool call.

[OBS] `XMLToolFormatter.FormatSystemPrompt` loads a separate 2,599-byte template
at `0x5ec49be`; see [`formatter-xml.txt`](evidence/system-prompt/formatter-xml.txt).
It contains a `%s` placeholder under the tool descriptions heading and requires
each tool name to be the surrounding XML tag with JSON arguments inside. It
conditionally appends the 2,257-byte
[`formatter-xml-examples.txt`](evidence/system-prompt/formatter-xml-examples.txt)
at `0x5eb102f`; the branches can also add the exact
[required-tool string](evidence/system-prompt/formatter-xml-required.txt) or
[named-required-tool format](evidence/system-prompt/formatter-xml-required-named.txt).
[INF] The formatted text is concatenated with the existing prompt; exact option
predicates require further tracing. Examples are binary literals, not proof that
those example paths, values or tools describe this machine or its active session.

[UNK] Per-model/account formatter selection, full request-field mutation, and
formatter templates for any other encoding remain unresolved. Formatter names
do not prove that every non-Gemini request uses prompt-encoded calls.
