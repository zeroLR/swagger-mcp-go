package versioning

import "context"

type versionedSpecKey struct{}

// withVersionedSpec adds a versioned spec to the context
func withVersionedSpec(ctx context.Context, spec *VersionedSpec) context.Context {
	return context.WithValue(ctx, versionedSpecKey{}, spec)
}

// GetVersionedSpecFromContext retrieves the versioned spec from the context
func GetVersionedSpecFromContext(ctx context.Context) (*VersionedSpec, bool) {
	spec, ok := ctx.Value(versionedSpecKey{}).(*VersionedSpec)
	return spec, ok
}