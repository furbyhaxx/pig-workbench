package fixture

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/furbyhaxx/pig-workbench/api"
)

type mutationNode struct{ text, kind string }

var mutationState = struct {
	sync.Mutex
	nodes    map[string]mutationNode
	receipts map[string]api.ResourceResult
	payloads map[string]string
}{nodes: map[string]mutationNode{}, receipts: map[string]api.ResourceResult{}, payloads: map[string]string{}}

func mutationRevision(node mutationNode) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(node.kind+":"+node.text)))
}

func executeMutation(request api.ResourceRequest) (any, *api.Error) {
	mutationState.Lock()
	defer mutationState.Unlock()
	node, exists := mutationState.nodes[request.URI]
	if request.URI == "mut://fixture/" || request.URI == "mut://fixture" {
		node, exists = mutationNode{kind: "dir"}, true
	}
	fail := func(code, text string) (any, *api.Error) {
		return nil, &api.Error{Code: code, Message: text, Outcome: "notApplied"}
	}
	switch request.Operation {
	case "stat":
		if !exists {
			return fail("notFound", "absent")
		}
		return api.ResourceResult{CanonicalURI: request.URI, Kind: node.kind, Revision: mutationRevision(node)}, nil
	case "list":
		if !exists || node.kind != "dir" {
			return fail("notFound", "not a directory")
		}
		prefix := strings.TrimRight(request.URI, "/") + "/"
		var entries []api.ResourceEntry
		for uri, node := range mutationState.nodes {
			rel := strings.TrimPrefix(uri, prefix)
			if rel != uri && !strings.Contains(rel, "/") {
				entries = append(entries, api.ResourceEntry{URI: uri, Name: rel, Kind: node.kind})
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		complete := true
		return api.ResourceResult{CanonicalURI: request.URI, Kind: "dir", Entries: entries, Complete: &complete}, nil
	case "read":
		if !exists {
			return fail("notFound", "absent")
		}
		count := strings.Count(node.text, "\n")
		if !strings.HasSuffix(node.text, "\n") && node.text != "" {
			count++
		}
		return api.ResourceResult{CanonicalURI: request.URI, Kind: node.kind, Revision: mutationRevision(node), Page: &api.TextPage{Text: node.text, ShownRanges: []api.LineRange{{Start: 1, End: count}}, TotalLines: &count, SelectionComplete: true, ResourceComplete: true}}, nil
	}
	m := request.Mutation
	if m == nil || m.AuthorizationRef != "router:"+request.OperationID || m.IdempotencyKey == "" {
		return fail("permissionDenied", "missing router binding")
	}
	encoded, _ := json.Marshal(request)
	if prior, ok := mutationState.receipts[m.IdempotencyKey]; ok {
		if mutationState.payloads[m.IdempotencyKey] != string(encoded) {
			return fail("conflict", "idempotency payload changed")
		}
		return prior, nil
	}
	if m.MustNotExist && exists || !m.MustNotExist && (!exists || m.ExpectedRevision != mutationRevision(node)) {
		return fail("staleRevision", "source changed")
	}
	if m.DestinationURI != "" {
		if _, exists := mutationState.nodes[m.DestinationURI]; exists || !m.DestinationMustNotExist {
			return fail("conflict", "destination exists")
		}
	}
	if m.MustNotExist || m.DestinationURI != "" {
		target := request.URI
		if m.DestinationURI != "" {
			target = m.DestinationURI
		}
		u, _ := url.Parse(target)
		parent := path.Dir(u.Path)
		if parent != "/" && parent != "." {
			u.Path = parent
			if node, ok := mutationState.nodes[u.String()]; !ok || node.kind != "dir" {
				return fail("notFound", "parent does not exist")
			}
		}
	}
	if strings.Contains(request.URI, "/deny") {
		return fail("permissionDenied", "provider policy denies this target")
	}
	status := "committed"
	effects := []api.MutationEffect{}
	changed := []string{}
	switch request.Operation {
	case "write":
		if m.Content == nil {
			return fail("invalidRequest", "missing content")
		}
		mutationState.nodes[request.URI] = mutationNode{text: *m.Content, kind: "file"}
		changed = append(changed, request.URI)
	case "mkdir":
		mutationState.nodes[request.URI] = mutationNode{kind: "dir"}
		changed = append(changed, request.URI)
	case "remove":
		for uri := range mutationState.nodes {
			if strings.HasPrefix(uri, strings.TrimRight(request.URI, "/")+"/") {
				return fail("conflict", "directory is not empty")
			}
		}
		delete(mutationState.nodes, request.URI)
		changed = append(changed, request.URI)
	case "copy", "move":
		mutationState.nodes[m.DestinationURI] = node
		changed = append(changed, m.DestinationURI)
		if request.Operation == "move" {
			delete(mutationState.nodes, request.URI)
			changed = append(changed, request.URI)
		}
	default:
		return fail("unsupported", "unknown mutation")
	}
	for _, uri := range changed {
		effects = append(effects, api.MutationEffect{URI: uri, Status: "committed"})
	}
	if strings.Contains(request.URI, "/unknown") {
		return nil, &api.Error{Code: "outcomeUnknown", Message: "reply lost", Outcome: "unknown"}
	}
	if strings.Contains(request.URI, "/partial") {
		status = "partial"
	}
	receipt := &api.MutationReceipt{OperationID: request.OperationID, IdempotencyKey: m.IdempotencyKey, Status: status, CanonicalURI: request.URI, Effects: effects, ChangedResources: changed}
	if strings.Contains(request.URI, "/invalid") {
		receipt.OperationID = "wrong"
	}
	result := api.ResourceResult{CanonicalURI: request.URI, Receipt: receipt}
	mutationState.receipts[m.IdempotencyKey] = result
	mutationState.payloads[m.IdempotencyKey] = string(encoded)
	return result, nil
}
