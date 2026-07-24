package prompt

import "embed"

// embeddedPromptsDir is the directory within the embed.FS containing the
// built-in prompt template files.
const embeddedPromptsDir = "templates"

// embeddedPromptsFS embeds all built-in prompt template JSON files shipped
// with the binary. Users can override or add templates by placing files in
// $MADY_HOME/prompt-templates/ without recompiling.
//
//go:embed templates/*.json templates/**/*.json
var embeddedPromptsFS embed.FS
