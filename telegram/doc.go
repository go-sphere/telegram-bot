// Package telegram builds Telegram bots on top of github.com/go-telegram/bot
// with command and callback routing, handler middleware, typed callback data
// and reply helpers.
//
// Create a bot with [NewApp], register handlers with [Bot.BindCommand],
// [Bot.BindCallback] and [Bot.BindNoRoute], then run long polling with
// [Bot.Start] until its context is cancelled. Handlers reply with
// [Bot.SendMessage], which sends a new message for a regular message and
// edits the original message for a callback query. Use [Bot.API] for any Bot
// API call this package does not wrap.
//
// # Usage
//
//	import (
//		"context"
//		"fmt"
//		"os"
//		"os/signal"
//
//		"github.com/go-sphere/telegram-bot/telegram"
//	)
//
//	app, err := telegram.NewApp(telegram.Config{Token: os.Getenv("TELEGRAM_BOT_TOKEN")})
//	if err != nil {
//		return err
//	}
//	app.BindCommand("start", func(ctx context.Context, update *telegram.Update) error {
//		return app.SendMessage(ctx, update, &telegram.Message{Text: "Hello!"})
//	})
//
//	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer stop()
//	return app.Start(ctx) // blocks until ctx is cancelled
//
// Register every handler before calling [Bot.Start]. Polling stops when the
// context passed to Start is cancelled; [Bot.Close] is not required to stop it.
//
// # Callback buttons
//
// Continuing the recipe above, [NewButton] encodes a route and a typed payload
// into callback data of the form "route:payload". A handler bound with
// [Bot.BindCallback] for the same route receives the press and decodes the
// payload with [UnmarshalData]:
//
//	type page struct {
//		N int `json:"n"` // only fields with a json tag are encoded
//	}
//
//	app.BindCommand("menu", func(ctx context.Context, update *telegram.Update) error {
//		btn, err := telegram.NewButton("Next", "page", page{N: 2})
//		if err != nil {
//			return err // empty route, ':' in route, or payload over 64 bytes
//		}
//		return app.SendMessage(ctx, update, &telegram.Message{
//			Text:   "Page 1",
//			Button: [][]telegram.Button{{btn}},
//		})
//	})
//
//	app.BindCallback("page", func(ctx context.Context, update *telegram.Update) error {
//		_, p, err := telegram.UnmarshalData[page](update.CallbackQuery.Data)
//		if err != nil {
//			return err
//		}
//		return app.SendMessage(ctx, update, &telegram.Message{Text: fmt.Sprint("Page ", p.N)})
//	})
//
// # Middleware and errors
//
// A [HandlerFunc] returns an error instead of replying to failures itself.
// Errors go to the [ErrorHandlerFunc] set with [WithErrorHandler] (by default
// they are logged with log/slog; [SendErrorMessage] can show them to the user).
// Middleware set with [AppendMiddlewares] runs first, then [NewAuthMiddleware]
// with the extractor set by [WithAuthExtractor], then middleware passed to an
// individual Bind call. Ready-made middleware includes
// [NewSingleFlightMiddleware] and [NewGroupMessageFilterMiddleware]. Handler
// panics are recovered and logged.
//
// Handlers run in their own goroutines by default, so several updates can be
// processed concurrently; pass github.com/go-telegram/bot.WithNotAsyncHandlers
// through [AppendBotOptions] to process updates synchronously.
//
// Related helpers: [BroadcastMessage] for rate-limited mass sending and
// [RetryOnTooManyRequestsErrorContext] for retrying after HTTP 429 responses.
// Mini App init data is validated by package
// github.com/go-sphere/telegram-bot/tmaauth.
package telegram
