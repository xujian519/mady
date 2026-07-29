package worker

// RegisterDefaultWorkers registers all default Workers into the given Registry.
// Pre-registered Workers are registered immediately; lazy Workers are skipped
// and can be activated later via LazyActivate.
func RegisterDefaultWorkers(registry *Registry, catalog *Catalog) []string {
	var skipped []string
	for _, d := range catalog.List() {
		if d.IsPreRegistered() {
			_ = registry.Register(d) // ignore duplicate errors on re-registration
		} else {
			skipped = append(skipped, d.Name)
		}
	}
	return skipped
}

// EnsureWorker is a convenience function that activates a Worker by name if it
// exists in the catalog but is not yet registered. Returns true if newly activated.
func EnsureWorker(registry *Registry, catalog *Catalog, name string) bool {
	d := catalog.Get(name)
	if d == nil {
		return false
	}
	if registry.Get(name) != nil {
		return false // already registered
	}
	return registry.LazyActivate(*d)
}
