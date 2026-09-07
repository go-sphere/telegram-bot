package telegram

import "context"

// ContextKey is the type of the context keys used by this package to carry
// per-update values (see AuthUserIDKey and AuthSubjectKey). Keeping a dedicated
// type avoids collisions with unrelated string keys in the same context chain.
type ContextKey string

// contextWithValues returns a context carrying every value of data under its
// own typed key. Map iteration order is not deterministic, which is harmless:
// the resulting chain of context.WithValue calls behaves identically for every
// order because each key is distinct.
func contextWithValues(ctx context.Context, data map[string]any) context.Context {
	for k, v := range data {
		ctx = context.WithValue(ctx, ContextKey(k), v)
	}
	return ctx
}
