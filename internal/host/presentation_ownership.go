package host

import (
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

type presentationToolsKey struct{}

func ownsTool(tools []sdk.ToolInfo, owner, name string) bool {
	for _, tool := range tools {
		if tool.Name == name && tool.Source == owner {
			return true
		}
	}
	return false
}

func verifyPresentationOwner(tools []sdk.ToolInfo, owner string, names []string) *api.Error {
	for _, name := range names {
		if !ownsTool(tools, owner, name) {
			return &api.Error{Code: "permissionDenied", Field: "params.descriptor.toolNames", Message: "verificationRequired: the presentation owner must match host tool source attribution", Outcome: "notApplied"}
		}
	}
	return nil
}
