package telegram

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// options holds configuration options for creating a Telegram bot application.
type options struct {
	noRouteHandler       bot.HandlerFunc  // Handler for unmatched routes
	errorHandler         ErrorHandlerFunc // Handler for processing errors
	authExtractor        AuthExtractorFunc
	deleteWebhookOnStart bool

	botOptions  []bot.Option     // Options to pass to the underlying bot client
	middlewares []MiddlewareFunc // Middleware functions to apply to handlers
}

// Option defines a function type for configuring bot application options.
// Options are applied by NewApp in order; a later option overrides an earlier
// one that sets the same value, while Append options accumulate.
type Option = func(*options)

func newOptions(opts ...Option) *options {
	defaults := &options{
		noRouteHandler: func(ctx context.Context, bot *bot.Bot, update *models.Update) {
			if update.Message != nil {
				slog.Info("receive message", slog.String("update", update.Message.Text))
			}
			if update.CallbackQuery != nil {
				slog.Info("receive callback query", slog.String("update", update.CallbackQuery.Data))
			}
		},
		errorHandler: func(ctx context.Context, bot *bot.Bot, update *Update, err error) {
			if update == nil {
				slog.Error("receive error", slog.Any("error", err))
				return
			}
			if update.Message != nil {
				slog.Error("receive message error",
					slog.Any("error", err),
					slog.String("text", update.Message.Text),
				)
				return
			}
			if update.CallbackQuery != nil {
				slog.Error("receive callback query error",
					slog.Any("error", err),
					slog.String("data", update.CallbackQuery.Data),
				)
				return
			}
			slog.Error("receive error", slog.Any("error", err))
		},
		authExtractor:        DefaultAuthExtractor,
		deleteWebhookOnStart: true,
		botOptions: []bot.Option{
			bot.WithSkipGetMe(),
			bot.WithMiddlewares(NewRecoveryMiddleware()),
		},
		middlewares: []MiddlewareFunc{},
	}
	for _, opt := range opts {
		opt(defaults)
	}
	return defaults
}

// WithErrorHandler sets a custom error handler for bot operations.
// The error handler will be called whenever a handler function returns an error,
// including an error from middleware such as the auth extractor. A nil fn
// discards errors. The default handler logs the error with log/slog.
func WithErrorHandler(fn ErrorHandlerFunc) Option {
	return func(o *options) {
		o.errorHandler = fn
	}
}

// WithDefaultHandler sets a custom handler for unmatched routes.
// This handler will be called when no specific command or callback query handler matches.
// fn is a raw github.com/go-telegram/bot handler: the middleware chain and
// error handler of this package are not applied. Use Bot.BindNoRoute for a
// HandlerFunc with middleware. The default logs the message text or callback
// data with log/slog.
func WithDefaultHandler(fn bot.HandlerFunc) Option {
	return func(o *options) {
		o.noRouteHandler = fn
	}
}

// WithAuthExtractor sets a custom authentication extractor for the bot.
// The extractor will be used to extract user information from incoming updates.
// It replaces DefaultAuthExtractor and runs for every handler registered with
// a Bind method; see NewAuthMiddleware for how its result reaches handlers.
func WithAuthExtractor(extractor AuthExtractorFunc) Option {
	return func(o *options) {
		o.authExtractor = extractor
	}
}

// WithDeleteWebhookOnStart controls whether Start deletes any previously
// configured webhook before beginning long polling. It is enabled by default so
// a leftover webhook cannot steal updates; disable it when a webhook is managed
// out of band or when Start is used alongside webhook mode.
func WithDeleteWebhookOnStart(enabled bool) Option {
	return func(o *options) {
		o.deleteWebhookOnStart = enabled
	}
}

// AppendBotOptions adds additional options to the underlying bot client configuration.
// These options will be passed directly to the bot.New() constructor, after
// the defaults (WithSkipGetMe, panic recovery middleware and the default
// handler), so they can override them.
func AppendBotOptions(opt ...bot.Option) Option {
	return func(o *options) {
		o.botOptions = append(o.botOptions, opt...)
	}
}

// AppendMiddlewares adds middleware functions to the bot application.
// Middleware will be applied to all handlers registered with a Bind method in
// the order they are provided: the first middleware is the outermost and runs
// before the auth middleware and before middleware passed to the Bind call.
func AppendMiddlewares(middlewares ...MiddlewareFunc) Option {
	return func(o *options) {
		o.middlewares = append(o.middlewares, middlewares...)
	}
}
