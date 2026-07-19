package doctmpl

import (
	"fmt"
	"sort"
)

// RendererRegistry maps OutputFormat values to Renderer implementations.
// It is safe for concurrent use once populated (all writes happen at
// initialization time).
type RendererRegistry struct {
	renderers map[OutputFormat]Renderer
}

// NewRendererRegistry creates an empty registry.
func NewRendererRegistry() *RendererRegistry {
	return &RendererRegistry{
		renderers: make(map[OutputFormat]Renderer),
	}
}

// Register adds a renderer for its declared format. Registering a format
// that already has a renderer overwrites the previous one.
func (r *RendererRegistry) Register(renderer Renderer) {
	if renderer == nil {
		return
	}
	r.renderers[renderer.Format()] = renderer
}

// Get returns the renderer for the given format. The second return value
// is false if no renderer is registered for that format.
func (r *RendererRegistry) Get(format OutputFormat) (Renderer, bool) {
	rend, ok := r.renderers[format]
	return rend, ok
}

// Has reports whether a renderer is registered for the given format.
func (r *RendererRegistry) Has(format OutputFormat) bool {
	_, ok := r.renderers[format]
	return ok
}

// Formats returns the list of registered formats, sorted alphabetically.
func (r *RendererRegistry) Formats() []OutputFormat {
	formats := make([]OutputFormat, 0, len(r.renderers))
	for f := range r.renderers {
		formats = append(formats, f)
	}
	sort.Slice(formats, func(i, j int) bool {
		return string(formats[i]) < string(formats[j])
	})
	return formats
}

// Render is a convenience method that looks up the renderer for the
// requested format and renders the Markdown body. It returns an error
// if no renderer is registered for the format.
func (r *RendererRegistry) Render(format OutputFormat, md string, meta RenderMeta) ([]byte, error) {
	rend, ok := r.Get(format)
	if !ok {
		return nil, fmt.Errorf("doctmpl: no renderer registered for format %q", format)
	}
	return rend.Render(md, meta)
}
