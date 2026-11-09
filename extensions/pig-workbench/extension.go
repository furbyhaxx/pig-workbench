// Package workbench is the pig-workbench PiG extension factory.
package workbench

import (
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/host"
)

// Name is the extension identity; it must match the extension directory and Package member.
const Name = "pig-workbench"

// Extension returns the pig-workbench extension factory.
func Extension() *sdk.Extension {
	ext := sdk.New(Name)
	host.Register(ext)
	return ext
}
