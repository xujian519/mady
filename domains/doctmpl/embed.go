package doctmpl

import "embed"

// embeddedTemplatesDir is the directory within the embed.FS containing the
// built-in template files.
const embeddedTemplatesDir = "templates"

// embeddedTemplatesFS embeds all built-in .md template files shipped with the
// binary. Users can override or add templates by placing files in
// $MADY_HOME/doc-templates/ without recompiling.
//
//go:embed templates/**/*.md
var embeddedTemplatesFS embed.FS
