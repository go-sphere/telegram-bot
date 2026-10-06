package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"
)

// AuthExtractor defines the interface for extracting authentication information from Telegram updates.
// Implementations should extract user identity and authorization data from the update context.
//
// ExtractorAuth returns values to store in the handler context: each map key
// is stored under ContextKey(key). A nil map adds nothing. A non-nil error
// stops the update before the handler runs and is passed to the bot's error
// handler. Implementations may be called concurrently for different updates.
type AuthExtractor interface {
	ExtractorAuth(ctx context.Context, update *Update) (map[string]any, error)
}

// AuthExtractorFunc is a function type that implements the AuthExtractor interface.
// It allows using functions as AuthExtractor implementations.
type AuthExtractorFunc func(ctx context.Context, update *Update) (map[string]any, error)

// ExtractorAuth implements the AuthExtractor interface by calling the function.
func (f AuthExtractorFunc) ExtractorAuth(ctx context.Context, update *Update) (map[string]any, error) {
	return f(ctx, update)
}

// Authentication context keys.
const (
	// AuthUserIDKey is the context key under which the authenticated user ID is stored.
	AuthUserIDKey ContextKey = "telegram.auth.uid"
	// AuthSubjectKey is the context key under which the authentication subject (username) is stored.
	AuthSubjectKey ContextKey = "telegram.auth.subject"
)

// AuthUserID returns the authenticated user ID extracted from the update, and
// whether it was present. The value is present only when the extractor stored
// an int64 under AuthUserIDKey, as DefaultAuthExtractor does.
func AuthUserID(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(AuthUserIDKey).(int64)
	return v, ok
}

// AuthSubject returns the authentication subject (username) extracted from the
// update, and whether it was present. The value is present only when the
// extractor stored a string under AuthSubjectKey; it may be empty for users
// without a username.
func AuthSubject(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(AuthSubjectKey).(string)
	return v, ok
}

// NewAuthMiddleware creates a middleware that extracts authentication information from updates.
// It uses the provided AuthExtractor to get user data and injects it into the request context.
// The extracted data becomes available to downstream handlers through
// ctx.Value(ContextKey(key)), AuthUserID and AuthSubject. If the extractor
// returns an error, the next handler is not called and the error is returned.
// NewApp installs this middleware with the extractor from WithAuthExtractor.
func NewAuthMiddleware(auth AuthExtractor) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, update *Update) error {
			info, err := auth.ExtractorAuth(ctx, update)
			if err != nil {
				return err
			}
			return next(contextWithValues(ctx, info), update)
		}
	}
}

// DefaultAuthExtractor is the default implementation for extracting authentication data from updates.
// It extracts user ID and username from either message or callback query updates,
// stored under AuthUserIDKey (int64) and AuthSubjectKey (string). It returns
// nil, nil for updates without a message sender or callback query, and never
// returns an error.
// The values are only advisory: they identify who sent the update but carry no
// proof of authenticity. Security-sensitive flows must validate the signature
// (for example with tmaauth) and authorize on the server side.
func DefaultAuthExtractor(ctx context.Context, update *Update) (map[string]any, error) {
	var user *models.User
	if update.Message != nil {
		user = update.Message.From
	}
	if update.CallbackQuery != nil {
		user = &update.CallbackQuery.From
	}
	if user == nil {
		return nil, nil
	}
	return map[string]any{
		string(AuthUserIDKey):  user.ID,
		string(AuthSubjectKey): user.Username,
	}, nil
}
