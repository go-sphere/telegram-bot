package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestAuthMiddlewareInjectsValues(t *testing.T) {
	update := &Update{Message: &models.Message{
		From: &models.User{ID: 42, Username: "alice"},
	}}
	mw := NewAuthMiddleware(AuthExtractorFunc(DefaultAuthExtractor))
	inner := mw(func(ctx context.Context, update *Update) error {
		uid, ok := AuthUserID(ctx)
		if !ok {
			t.Error("user id missing from context")
		}
		if uid != 42 {
			t.Errorf("uid = %d, want 42", uid)
		}
		subject, ok := AuthSubject(ctx)
		if !ok {
			t.Error("subject missing from context")
		}
		if subject != "alice" {
			t.Errorf("subject = %q, want alice", subject)
		}
		return nil
	})
	if err := inner(context.Background(), update); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
}

func TestAuthMiddlewareFromCallback(t *testing.T) {
	update := &Update{CallbackQuery: &models.CallbackQuery{
		From: models.User{ID: 7, Username: "bob"},
	}}
	mw := NewAuthMiddleware(AuthExtractorFunc(DefaultAuthExtractor))
	called := false
	inner := mw(func(ctx context.Context, update *Update) error {
		called = true
		uid, ok := AuthUserID(ctx)
		if !ok || uid != 7 {
			t.Errorf("uid = %d (ok=%v), want 7", uid, ok)
		}
		return nil
	})
	if err := inner(context.Background(), update); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !called {
		t.Fatal("handler not called")
	}
}

func TestAuthMiddlewareNoUserNoValues(t *testing.T) {
	update := &Update{Message: &models.Message{Text: "no sender"}}
	mw := NewAuthMiddleware(AuthExtractorFunc(DefaultAuthExtractor))
	inner := mw(func(ctx context.Context, update *Update) error {
		if _, ok := AuthUserID(ctx); ok {
			t.Error("unexpected user id")
		}
		if _, ok := AuthSubject(ctx); ok {
			t.Error("unexpected subject")
		}
		return nil
	})
	if err := inner(context.Background(), update); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
}

func TestAuthMiddlewareErrorStopsChain(t *testing.T) {
	sentinel := errors.New("auth failed")
	mw := NewAuthMiddleware(AuthExtractorFunc(func(ctx context.Context, update *Update) (map[string]any, error) {
		return nil, sentinel
	}))
	called := false
	inner := mw(func(ctx context.Context, update *Update) error {
		called = true
		return nil
	})
	err := inner(context.Background(), &Update{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
	if called {
		t.Error("handler must not run when auth extraction fails")
	}
}

func TestContextKeyTypeCollision(t *testing.T) {
	// The dedicated ContextKey must not collide with an unrelated plain string
	// key of the same spelling in the parent context.
	type unrelatedStringKey string
	parent := context.WithValue(context.Background(), unrelatedStringKey("telegram.auth.uid"), "not-an-int64")
	ctx := contextWithValues(parent, map[string]any{
		string(AuthUserIDKey): int64(9),
	})
	if v, _ := AuthUserID(ctx); v != 9 {
		t.Errorf("AuthUserID = %d, want 9", v)
	}
}

func TestContextValueFallsThroughToParent(t *testing.T) {
	type parentKey struct{}
	parent := context.WithValue(context.Background(), parentKey{}, "parent-value")
	ctx := contextWithValues(parent, map[string]any{"a": "b"})

	if got := ctx.Value(parentKey{}); got != "parent-value" {
		t.Errorf("parent value not reachable: %v", got)
	}
	if got := ctx.Value(ContextKey("a")); got != "b" {
		t.Errorf("own value not reachable: %v", got)
	}
	if got := ctx.Value("a"); got != nil {
		t.Errorf("plain string key must not collide with the typed key, got %v", got)
	}
	if _, ok := AuthUserID(ctx); ok {
		t.Error("accessor must return not-ok when value is missing")
	}
}

func TestContextWithValuesNilData(t *testing.T) {
	parent := context.Background()
	ctx := contextWithValues(parent, nil)
	if ctx != parent {
		t.Error("nil data must return the parent context unchanged")
	}
	ctx = contextWithValues(parent, map[string]any{})
	if ctx != parent {
		t.Error("empty data must return the parent context unchanged")
	}
}
