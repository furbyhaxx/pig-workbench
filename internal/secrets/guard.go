package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TextGuard transforms model-facing text. *Detector implements it; the
// interface exists so a boundary can be exercised with a failing guard.
type TextGuard interface {
	// Redact replaces known secret values with their placeholders.
	Redact(text string) (string, error)
	// Substitute replaces canonical placeholders with their values.
	Substitute(text string) (string, error)
}

// ErrUnsupportedShape reports model-facing content the boundary cannot classify.
// Path names a structural location such as "messages[2].content[0].type"; the
// message never carries content or a guard diagnostic.
type ErrUnsupportedShape struct {
	Path string
}

// Error describes the uncovered shape without including content.
func (e *ErrUnsupportedShape) Error() string {
	return "secret boundary does not support the payload shape at " + e.Path
}

// ErrGuardFailed reports a refused guard call. It deliberately keeps no cause,
// so a detector message or its text cannot reach an error string.
type ErrGuardFailed struct {
	Path string
}

// Error describes the refused call without including the guard diagnostic.
func (e *ErrGuardFailed) Error() string {
	return "secret boundary unavailable at " + e.Path
}

var errNoGuard = errors.New("secret boundary requires a text guard")

// RedactContent redacts a string or native content blocks. Text blocks are
// redacted; image, audio and file blocks are binary payloads and stay intact.
// Malformed or unknown content fails closed instead of being stringified.
func RedactContent(guard TextGuard, content any) (any, error) {
	if guard == nil {
		return nil, errNoGuard
	}
	return transformContent(guard.Redact, content, "content")
}

// SubstituteArguments substitutes string values in tool arguments recursively.
// Keys, numbers and other scalars are never transformed: argument objects are
// user data, so no key is exempt from substitution but no key is rewritten either.
func SubstituteArguments(guard TextGuard, args map[string]any) (map[string]any, error) {
	if guard == nil {
		return nil, errNoGuard
	}
	transformed, err := transformValues(guard.Substitute, args, "arguments")
	if err != nil {
		return nil, err
	}
	out, ok := transformed.(map[string]any)
	if !ok {
		return nil, &ErrUnsupportedShape{Path: "arguments"}
	}
	return out, nil
}

// RedactMessages redacts model-facing text in SDK and OpenAI messages. role,
// name, id and tool_call_id stay verbatim, so a stored short value that happens
// to equal "user" or "read" can never rewrite a protocol string. Content is a
// string or blocks; tool_calls and function_call arguments are parsed as JSON,
// redacted by value and serialized again.
func RedactMessages(guard TextGuard, messages []any) ([]any, error) {
	if guard == nil {
		return nil, errNoGuard
	}
	return transformMessages(guard.Redact, messages, "messages")
}

// RedactProviderPayload redacts the text-bearing positions of an OpenAI chat
// completions, OpenAI responses, Anthropic messages or Gemini request payload.
// Supported positions: messages, system, instructions, input, contents,
// systemInstruction/system_instruction, tools and message-level tools. Every
// other top-level key is provider metadata and is copied verbatim. A non-empty
// payload holding none of those positions is rejected as uncovered, so an
// unknown dialect can never pass as protected.
func RedactProviderPayload(guard TextGuard, payload map[string]any) (map[string]any, error) {
	if guard == nil {
		return nil, errNoGuard
	}
	transform := guard.Redact
	out := make(map[string]any, len(payload))
	covered := 0
	for key, value := range payload {
		switch key {
		case "messages":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: key}
			}
			transformed, err := transformMessages(transform, list, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "input":
			transformed, err := transformInput(transform, value, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "system", "instructions":
			transformed, err := transformContent(transform, value, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "contents":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: key}
			}
			transformed, err := transformGeminiContents(transform, list, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "systemInstruction", "system_instruction":
			transformed, err := transformGeminiInstruction(transform, value, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "tools":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: key}
			}
			transformed, err := transformToolDefinitions(transform, list, key)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		default:
			copied, err := deepCopy(value, key)
			if err != nil {
				return nil, err
			}
			out[key] = copied
			continue
		}
		covered++
	}
	if covered == 0 && len(payload) > 0 {
		return nil, &ErrUnsupportedShape{Path: "payload"}
	}
	return out, nil
}

// textOp is one guard method, so redaction and substitution share the walkers.
type textOp func(string) (string, error)

func applyText(transform textOp, path, text string) (string, error) {
	out, err := transform(text)
	if err != nil {
		return "", &ErrGuardFailed{Path: path}
	}
	return out, nil
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func indexPath(path string, index int) string {
	return fmt.Sprintf("%s[%d]", path, index)
}

// deepCopy copies JSON kinds without transforming them and rejects anything
// else, so an unexpected Go type can never reach a provider as unredacted text.
func deepCopy(value any, path string) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		return typed, nil
	case bool:
		return typed, nil
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			copied, err := deepCopy(item, joinPath(path, key))
			if err != nil {
				return nil, err
			}
			out[key] = copied
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			copied, err := deepCopy(item, indexPath(path, index))
			if err != nil {
				return nil, err
			}
			out[index] = copied
		}
		return out, nil
	default:
		return nil, &ErrUnsupportedShape{Path: path}
	}
}

func copyObject(value map[string]any, path string) (map[string]any, error) {
	copied, err := deepCopy(value, path)
	if err != nil {
		return nil, err
	}
	return copied.(map[string]any), nil
}

// transformValues transforms string values only. Keys are structure, never text.
func transformValues(transform textOp, value any, path string) (any, error) {
	switch typed := value.(type) {
	case string:
		return applyText(transform, path, typed)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			transformed, err := transformValues(transform, item, joinPath(path, key))
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			transformed, err := transformValues(transform, item, indexPath(path, index))
			if err != nil {
				return nil, err
			}
			out[index] = transformed
		}
		return out, nil
	default:
		return deepCopy(value, path)
	}
}

// textBlockKind reports whether kind is a content block that carries only text.
func textBlockKind(kind string) bool {
	return kind == "text" || kind == "input_text" || kind == "output_text"
}

// opaqueBlockKind reports blocks that carry binary or provider-owned payloads:
// token patterns must never run across base64 image or audio bytes.
func opaqueBlockKind(kind string) bool {
	switch kind {
	case "image", "image_url", "input_image", "input_audio", "audio", "file", "input_file", "document":
		return true
	}
	return false
}

func transformContent(transform textOp, content any, path string) (any, error) {
	switch typed := content.(type) {
	case nil:
		return nil, nil
	case string:
		return applyText(transform, path, typed)
	case []any:
		blocks := make([]any, len(typed))
		for index, raw := range typed {
			blockPath := indexPath(path, index)
			block, ok := raw.(map[string]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: blockPath}
			}
			transformed, err := transformBlock(transform, block, blockPath)
			if err != nil {
				return nil, err
			}
			blocks[index] = transformed
		}
		return blocks, nil
	default:
		return nil, &ErrUnsupportedShape{Path: path}
	}
}

func transformBlock(transform textOp, block map[string]any, path string) (map[string]any, error) {
	kind, ok := block["type"].(string)
	if !ok {
		return nil, &ErrUnsupportedShape{Path: joinPath(path, "type")}
	}
	switch {
	case textBlockKind(kind):
		text, ok := block["text"].(string)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: joinPath(path, "text")}
		}
		transformed, err := applyText(transform, joinPath(path, "text"), text)
		if err != nil {
			return nil, err
		}
		copied, err := copyObject(block, path)
		if err != nil {
			return nil, err
		}
		copied["text"] = transformed
		return copied, nil
	case opaqueBlockKind(kind):
		return copyObject(block, path)
	case kind == "tool_use", kind == "toolCall":
		argumentsKey := "input"
		if kind == "toolCall" {
			argumentsKey = "arguments"
		}
		copied, err := copyObject(block, path)
		if err != nil {
			return nil, err
		}
		if input, present := block[argumentsKey]; present {
			transformed, err := transformValues(transform, input, joinPath(path, argumentsKey))
			if err != nil {
				return nil, err
			}
			copied[argumentsKey] = transformed
		}
		return copied, nil
	case kind == "tool_result":
		copied, err := copyObject(block, path)
		if err != nil {
			return nil, err
		}
		if content, present := block["content"]; present {
			transformed, err := transformContent(transform, content, joinPath(path, "content"))
			if err != nil {
				return nil, err
			}
			copied["content"] = transformed
		}
		return copied, nil
	case kind == "thinking", kind == "redacted_thinking":
		// Model reasoning output; it cannot carry a value the model never held.
		return copyObject(block, path)
	default:
		return nil, &ErrUnsupportedShape{Path: path}
	}
}

func transformMessages(transform textOp, messages []any, path string) ([]any, error) {
	out := make([]any, len(messages))
	for index, raw := range messages {
		messagePath := indexPath(path, index)
		message, ok := raw.(map[string]any)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: messagePath}
		}
		transformed, err := transformMessage(transform, message, messagePath)
		if err != nil {
			return nil, err
		}
		out[index] = transformed
	}
	return out, nil
}

func transformMessage(transform textOp, message map[string]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(message))
	for key, value := range message {
		fieldPath := joinPath(path, key)
		switch key {
		case "content", "output":
			transformed, err := transformContent(transform, value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "reasoning_content", "reasoning", "reasoning_text":
			text, ok := value.(string)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := applyText(transform, fieldPath, text)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "arguments":
			transformed, err := transformArgumentsJSON(transform, value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "tool_calls":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformToolCalls(transform, list, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "function_call":
			call, ok := value.(map[string]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformToolObject(transform, call, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "tools":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformToolDefinitions(transform, list, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		default:
			copied, err := deepCopy(value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = copied
		}
	}
	return out, nil
}

func transformInput(transform textOp, input any, path string) (any, error) {
	if list, ok := input.([]any); ok {
		return transformMessages(transform, list, path)
	}
	return transformContent(transform, input, path)
}

func transformToolCalls(transform textOp, calls []any, path string) ([]any, error) {
	out := make([]any, len(calls))
	for index, raw := range calls {
		callPath := indexPath(path, index)
		call, ok := raw.(map[string]any)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: callPath}
		}
		transformed, err := transformToolCall(transform, call, callPath)
		if err != nil {
			return nil, err
		}
		out[index] = transformed
	}
	return out, nil
}

func transformToolCall(transform textOp, call map[string]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(call))
	for key, value := range call {
		fieldPath := joinPath(path, key)
		switch key {
		case "function", "custom":
			object, ok := value.(map[string]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformToolObject(transform, object, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		default:
			copied, err := deepCopy(value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = copied
		}
	}
	return out, nil
}

func transformToolDefinitions(transform textOp, tools []any, path string) ([]any, error) {
	out := make([]any, len(tools))
	for index, raw := range tools {
		toolPath := indexPath(path, index)
		tool, ok := raw.(map[string]any)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: toolPath}
		}
		transformed, err := transformToolObject(transform, tool, toolPath)
		if err != nil {
			return nil, err
		}
		out[index] = transformed
	}
	return out, nil
}

// transformToolObject covers a tool definition, a tool call's function object,
// a Gemini declaration and an OpenAI custom tool. name stays verbatim because it
// identifies the tool; description is prose and is redacted.
func transformToolObject(transform textOp, object map[string]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(object))
	for key, value := range object {
		fieldPath := joinPath(path, key)
		switch key {
		case "description":
			text, ok := value.(string)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := applyText(transform, fieldPath, text)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "arguments":
			transformed, err := transformArgumentsJSON(transform, value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "input":
			// A streamed grammar tool call carries raw input text, not JSON.
			text, ok := value.(string)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := applyText(transform, fieldPath, text)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "parameters", "input_schema":
			transformed, err := transformSchema(transform, value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "function", "custom":
			nested, ok := value.(map[string]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformToolObject(transform, nested, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		case "functionDeclarations", "function_declarations":
			list, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			declarations := make([]any, len(list))
			for index, raw := range list {
				declaration, ok := raw.(map[string]any)
				if !ok {
					return nil, &ErrUnsupportedShape{Path: indexPath(fieldPath, index)}
				}
				transformed, err := transformToolObject(transform, declaration, indexPath(fieldPath, index))
				if err != nil {
					return nil, err
				}
				declarations[index] = transformed
			}
			out[key] = declarations
		default:
			copied, err := deepCopy(value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = copied
		}
	}
	return out, nil
}

// transformSchema redacts textual schema descriptions only. Property names,
// enums, defaults, patterns and formats are contract values the tool expects
// back, so replacing them would change what the model is told to send.
func transformSchema(transform textOp, value any, path string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			fieldPath := joinPath(path, key)
			// A property may itself be named "description"; only a string here is prose.
			if text, ok := item.(string); ok && key == "description" {
				transformed, err := applyText(transform, fieldPath, text)
				if err != nil {
					return nil, err
				}
				out[key] = transformed
				continue
			}
			transformed, err := transformSchema(transform, item, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			transformed, err := transformSchema(transform, item, indexPath(path, index))
			if err != nil {
				return nil, err
			}
			out[index] = transformed
		}
		return out, nil
	default:
		return deepCopy(value, path)
	}
}

// transformArgumentsJSON parses tool arguments, transforms their string values
// and serializes them again. Numbers keep their literal form through json.Number.
func transformArgumentsJSON(transform textOp, raw any, path string) (any, error) {
	text, ok := raw.(string)
	if !ok {
		return nil, &ErrUnsupportedShape{Path: path}
	}
	if text == "" {
		return text, nil
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, &ErrUnsupportedShape{Path: path}
	}
	if decoder.More() {
		return nil, &ErrUnsupportedShape{Path: path}
	}
	transformed, err := transformValues(transform, decoded, path)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(transformed)
	if err != nil {
		return nil, &ErrUnsupportedShape{Path: path}
	}
	return string(encoded), nil
}

func transformGeminiContents(transform textOp, contents []any, path string) ([]any, error) {
	out := make([]any, len(contents))
	for index, raw := range contents {
		contentPath := indexPath(path, index)
		content, ok := raw.(map[string]any)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: contentPath}
		}
		transformed := make(map[string]any, len(content))
		for key, value := range content {
			fieldPath := joinPath(contentPath, key)
			if key != "parts" {
				copied, err := deepCopy(value, fieldPath)
				if err != nil {
					return nil, err
				}
				transformed[key] = copied
				continue
			}
			parts, ok := value.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformedParts, err := transformGeminiParts(transform, parts, fieldPath)
			if err != nil {
				return nil, err
			}
			transformed[key] = transformedParts
		}
		out[index] = transformed
	}
	return out, nil
}

// transformGeminiInstruction covers systemInstruction, which is a string or an
// object holding parts. system_instruction wraps its parts the same way.
func transformGeminiInstruction(transform textOp, value any, path string) (any, error) {
	switch typed := value.(type) {
	case string:
		return applyText(transform, path, typed)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			fieldPath := joinPath(path, key)
			if key != "parts" {
				copied, err := deepCopy(item, fieldPath)
				if err != nil {
					return nil, err
				}
				out[key] = copied
				continue
			}
			parts, ok := item.([]any)
			if !ok {
				return nil, &ErrUnsupportedShape{Path: fieldPath}
			}
			transformed, err := transformGeminiParts(transform, parts, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = transformed
		}
		return out, nil
	default:
		return nil, &ErrUnsupportedShape{Path: path}
	}
}

func transformGeminiParts(transform textOp, parts []any, path string) ([]any, error) {
	out := make([]any, len(parts))
	for index, raw := range parts {
		partPath := indexPath(path, index)
		part, ok := raw.(map[string]any)
		if !ok {
			return nil, &ErrUnsupportedShape{Path: partPath}
		}
		transformed, err := transformGeminiPart(transform, part, partPath)
		if err != nil {
			return nil, err
		}
		out[index] = transformed
	}
	return out, nil
}

func transformGeminiPart(transform textOp, part map[string]any, path string) (map[string]any, error) {
	if text, ok := part["text"].(string); ok {
		transformed, err := applyText(transform, joinPath(path, "text"), text)
		if err != nil {
			return nil, err
		}
		copied, err := copyObject(part, path)
		if err != nil {
			return nil, err
		}
		copied["text"] = transformed
		return copied, nil
	}
	for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data"} {
		if _, present := part[key]; present {
			return copyObject(part, path)
		}
	}
	for _, key := range []string{"functionCall", "function_call", "functionResponse", "function_response"} {
		call, ok := part[key].(map[string]any)
		if !ok {
			continue
		}
		transformed, err := transformGeminiCall(transform, call, joinPath(path, key))
		if err != nil {
			return nil, err
		}
		copied, err := copyObject(part, path)
		if err != nil {
			return nil, err
		}
		copied[key] = transformed
		return copied, nil
	}
	return nil, &ErrUnsupportedShape{Path: path}
}

func transformGeminiCall(transform textOp, call map[string]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(call))
	for key, value := range call {
		fieldPath := joinPath(path, key)
		if key != "args" && key != "response" {
			copied, err := deepCopy(value, fieldPath)
			if err != nil {
				return nil, err
			}
			out[key] = copied
			continue
		}
		transformed, err := transformValues(transform, value, fieldPath)
		if err != nil {
			return nil, err
		}
		out[key] = transformed
	}
	return out, nil
}
