// Package prompt renders typed system prompt inputs with Jinja templates.
package prompt

// Data is the request-stable input available to a template.
type Data struct {
	Model        Model
	Tools        Tools
	Host         Host
	Instructions []Instruction
	Workbench    Workbench
	ProjectRoot  string
}

// Model contains the selected model's fixed prompt properties.
type Model struct {
	Provider       string `jinja:"provider"`
	ID             string `jinja:"id"`
	Name           string `jinja:"name"`
	API            string `jinja:"api"`
	Family         string `jinja:"family"`
	Reasoning      bool   `jinja:"reasoning"`
	ContextWindow  int    `jinja:"context_window"`
	MaxOutput      int    `jinja:"max_output"`
	Thinking       string `jinja:"thinking"`
	MidConvoSystem bool   `jinja:"mid_convo_system"`
}

// Tools describes the ordered loadout and its prompt contributions.
type Tools struct {
	Selected   []string
	Declared   []string
	Hidden     []string
	Snippets   map[string]string
	Guidelines map[string][]string
}

// Host contains the host's prompt options and execution environment.
type Host struct {
	CustomPrompt     string
	Append           string
	PromptGuidelines []string
	Docs             bool
	ContextFiles     []ContextFile
	Skills           []HostSkill
	Sections         map[string]string
	Mode             string
	OS               string
	Shell            string
}

// ContextFile is a host-supplied instruction file.
type ContextFile struct {
	Path    string `jinja:"path"`
	Content string `jinja:"content"`
}

// HostSkill is the metadata supplied by PiG for a discovered skill.
type HostSkill struct {
	Name                   string     `jinja:"name"`
	Description            string     `jinja:"description"`
	FilePath               string     `jinja:"file_path"`
	BaseDir                string     `jinja:"base_dir"`
	SourceInfo             SourceInfo `jinja:"source_info"`
	DisableModelInvocation bool       `jinja:"disable_model_invocation"`
}

// SourceInfo identifies the origin of a host-discovered skill.
type SourceInfo struct {
	Path    string `jinja:"path"`
	Source  string `jinja:"source"`
	Scope   string `jinja:"scope"`
	Origin  string `jinja:"origin"`
	BaseDir string `jinja:"base_dir"`
}

// Instruction is one stable instruction selected for the request.
type Instruction struct {
	Class       string `jinja:"class"`
	Path        string `jinja:"path"`
	DisplayPath string `jinja:"display_path"`
	Role        string `jinja:"role"`
	Content     string `jinja:"content"`
}

// Workbench contains prompt-relevant feature settings.
type Workbench struct {
	Edit    EditFeatures   `jinja:"edit"`
	Read    ReadFeatures   `jinja:"read"`
	Secrets SecretFeatures `jinja:"secrets"`
	Schemes []Scheme       `jinja:"schemes"`
}

// EditFeatures contains the selected edit behavior.
type EditFeatures struct {
	Mode string `jinja:"mode"`
}

// ReadFeatures identifies enabled read capabilities.
type ReadFeatures struct {
	Summarize bool `jinja:"summarize"`
	Selectors bool `jinja:"selectors"`
}

// SecretFeatures contains the secret-placeholder policy.
type SecretFeatures struct {
	Enabled bool `jinja:"enabled"`
}

// Scheme identifies a registered resource scheme.
type Scheme struct {
	Name    string `jinja:"name"`
	Summary string `jinja:"summary"`
}
