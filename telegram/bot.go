package telegram

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Config defines the configuration parameters for the Telegram bot.
type Config struct {
	// Token is the bot token issued by @BotFather. It must not be empty or
	// whitespace only; NewApp rejects such a token.
	Token string `json:"token" yaml:"token"`
}

// Bot represents a Telegram bot application with routing and middleware support.
// It wraps the underlying bot client and provides additional functionality for
// handling messages, authentication, and error processing.
//
// Create a Bot with NewApp; the zero value is not usable. Register handlers
// (BindCommand, BindCallback, BindNoRoute, BindRoute) and apply Update before
// calling Start: BindNoRoute and Update are not synchronized with update
// processing. After Close, the Bot must not be used again.
type Bot struct {
	config Config
	bot    *bot.Bot

	deleteWebhookOnStart bool

	middlewares    []MiddlewareFunc
	noRouteHandler bot.HandlerFunc
	errorHandler   ErrorHandlerFunc
	authExtractor  AuthExtractorFunc
}

// NewApp creates a new Telegram bot application with the provided configuration and options.
// It initializes the bot client, applies middleware, and sets up default handlers.
//
// NewApp makes no network request: the underlying client is created with
// github.com/go-telegram/bot.WithSkipGetMe, so an invalid but non-empty token is
// only detected by the first API call. It returns an error when config.Token
// is empty or whitespace only, or when an option passed through
// AppendBotOptions makes client creation fail (for example by removing
// WithSkipGetMe and failing the getMe call).
//
// Defaults: unmatched updates are logged, handler errors are logged with
// log/slog, DefaultAuthExtractor fills the context, handler panics are
// recovered and logged, and Start deletes any configured webhook first.
func NewApp(config Config, opts ...Option) (*Bot, error) {
	opt := newOptions(opts...)
	app := &Bot{
		config:               config,
		deleteWebhookOnStart: opt.deleteWebhookOnStart,
		middlewares:          opt.middlewares,
		noRouteHandler:       opt.noRouteHandler,
		errorHandler:         opt.errorHandler,
		authExtractor:        opt.authExtractor,
	}
	opt.botOptions = append(opt.botOptions,
		bot.WithDefaultHandler(
			func(ctx context.Context, bot *bot.Bot, update *models.Update) {
				app.noRouteHandler(ctx, bot, update)
			},
		),
	)
	app.middlewares = append(app.middlewares,
		NewAuthMiddleware(
			AuthExtractorFunc(func(ctx context.Context, update *Update) (map[string]any, error) {
				return app.authExtractor(ctx, update)
			}),
		),
	)
	client, err := bot.New(config.Token, opt.botOptions...)
	if err != nil {
		return nil, err
	}
	app.bot = client
	return app, nil
}

// Update applies additional configuration options to the underlying bot client.
// Call it before Start; options are applied directly without synchronization.
func (b *Bot) Update(options ...bot.Option) {
	for _, opt := range options {
		opt(b.bot)
	}
}

// API returns the underlying Telegram bot client for direct API access.
// It returns nil after Close.
func (b *Bot) API() *bot.Bot {
	return b.bot
}

// Start begins the bot's update polling and message processing.
// Unless disabled with WithDeleteWebhookOnStart(false), it first removes any
// existing webhook so a leftover webhook cannot steal updates, then starts long
// polling. It blocks until ctx is cancelled. The webhook removal error, if any,
// is returned instead of being swallowed.
//
// Start returns nil after ctx is cancelled and the polling and worker
// goroutines have stopped; cancelling ctx is how a running bot is stopped.
// Handlers that were started asynchronously may still be running when Start
// returns. Polling errors are reported through the underlying client's error
// handler (see github.com/go-telegram/bot.WithErrorsHandler), not returned.
func (b *Bot) Start(ctx context.Context) error {
	if b.deleteWebhookOnStart {
		// nil params so go-telegram/bot sends an empty body instead of a
		// boundary-only multipart form, which Telegram rejects with 400.
		if _, err := b.bot.DeleteWebhook(ctx, nil); err != nil {
			return err
		}
	}
	b.bot.Start(ctx)
	return nil
}

// Close calls the Telegram Bot API "close" method, then detaches the client
// from b. It is not a cleanup method and must not be deferred as one: the Bot
// API close method is only for moving a bot from one Bot API server to
// another, Telegram rejects it with 429 for the first 10 minutes after the bot
// is launched, and it does not stop polling (cancel the context passed to
// Start for that). Normal shutdown needs no call at all.
//
// The client is detached even when the request fails, so API returns nil and
// other methods must not be called afterwards. Calling Close again is a no-op
// that returns nil.
func (b *Bot) Close(ctx context.Context) error {
	if b.bot == nil {
		return nil
	}
	_, err := b.bot.Close(ctx)
	b.bot = nil
	return err
}

// SendMessage sends a message in response to an update using the bot's client.
// It behaves like the package-level SendMessage: a regular message gets a new
// reply in the same chat, and a callback query edits the message that carried
// the button. It returns nil without sending when update or m is nil.
func (b *Bot) SendMessage(ctx context.Context, update *Update, m *Message) error {
	return SendMessage(ctx, b.bot, update, m)
}

func (b *Bot) appendMiddlewares(middlewares ...MiddlewareFunc) []MiddlewareFunc {
	mid := make([]MiddlewareFunc, 0, len(middlewares)+len(b.middlewares))
	mid = append(mid, b.middlewares...)
	mid = append(mid, middlewares...)
	return mid
}

// BindNoRoute sets the default handler for messages that don't match any specific route.
// This handler is called when no command or callback query handlers match the incoming update.
// It replaces the handler set with WithDefaultHandler and, unlike that option,
// runs the bot middleware and error handler around handlerFunc. Call it
// before Start.
func (b *Bot) BindNoRoute(handlerFunc HandlerFunc, middlewares ...MiddlewareFunc) {
	b.noRouteHandler = WithMiddleware(handlerFunc, b.errorHandler, b.appendMiddlewares(middlewares...)...)
}

// matchCommandMessage reports whether msg starts with the given slash command,
// optionally carrying an "@username" suffix (for example "/start" or
// "/start@MyBot"). The match is token based so "/start" never matches "/startup"
// or "foo /start". Commands addressed to another bot (e.g. "/start@OtherBot")
// still match here: in group chats NewGroupMessageFilterMiddleware drops them
// before the handler runs, while the bot's own username is not known without an
// extra GetMe call.
func matchCommandMessage(msg *models.Message, command string) bool {
	if msg == nil {
		return false
	}
	command = strings.TrimPrefix(command, "/")
	token := msg.Text
	if i := strings.IndexAny(token, " \t\n"); i >= 0 {
		token = token[:i]
	}
	if token == "/"+command {
		return true
	}
	return strings.HasPrefix(token, "/"+command+"@")
}

// BindCommand registers a handler for a specific bot command (e.g., "/start", "/help").
// The command parameter should not include the leading slash, as it will be added automatically.
// Commands match on the first word of a message: "/start" matches "/start" and
// "/start@BotName", but not "/startup".
//
// handlerFunc is wrapped with the bot middleware (AppendMiddlewares, then the
// auth middleware), followed by middlewares, and its error is passed to the
// bot's error handler. Handlers are tried in registration order and the first
// match wins.
func (b *Bot) BindCommand(command string, handlerFunc HandlerFunc, middlewares ...MiddlewareFunc) {
	fn := WithMiddleware(handlerFunc, b.errorHandler, b.appendMiddlewares(middlewares...)...)
	command = strings.TrimPrefix(command, "/")
	b.bot.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchCommandMessage(update.Message, command)
	}, fn)
}

// BindCallback registers a handler for callback query data with a specific route prefix.
// The handler matches callback data starting with route followed by ":", so
// BindCallback("menu", ...) matches "menu:item1" produced by
// NewButton(text, "menu", data) or MarshalData("menu", data).
// Middleware and error handling are applied as in BindCommand.
func (b *Bot) BindCallback(route string, handlerFunc HandlerFunc, middlewares ...MiddlewareFunc) {
	fn := WithMiddleware(handlerFunc, b.errorHandler, b.appendMiddlewares(middlewares...)...)
	b.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, route+":", bot.MatchTypePrefix, fn)
}

// MessageSender defines a function that sends messages in response to updates.
// Bot.SendMessage has this signature.
type MessageSender = func(ctx context.Context, request *Update, msg *Message) error

// RouteMap maps operation names to their corresponding handler functions.
type RouteMap = map[string]func(ctx context.Context, request *Update) error

// RouteMapBuilder defines a function that builds a RouteMap from a service, codec, and message sender.
// The Register<Service>RouteServer functions generated by
// github.com/go-sphere/protoc-gen-route have this shape; pass their result to
// Bot.BindRoute.
type RouteMapBuilder[S any, D any] = func(srv S, codec D, sender MessageSender) RouteMap

// BindRoute registers multiple handlers based on a route map and method metadata.
// It automatically binds commands and callback queries based on the provided operation metadata.
//
// For every name in operations, extra must return non-nil metadata; a
// non-empty Command is bound with BindCommand and a non-empty CallbackQuery
// with BindCallback, both using route[name] and middlewares. route must
// contain a handler for every operation that is bound.
//
// With code generated by protoc-gen-route for MenuService in package menuv1:
//
//	routes := menuv1.RegisterMenuServiceRouteServer(srv, codec, app.SendMessage)
//	app.BindRoute(routes,
//		menuv1.GetExtraRouteDataByMenuServiceOperation,
//		menuv1.GetAllRouteMenuServiceOperations(),
//	)
func (b *Bot) BindRoute(route RouteMap, extra func(string) *MethodExtraData, operations []string, middlewares ...MiddlewareFunc) {
	for _, operation := range operations {
		info := extra(operation)
		if info.Command != "" {
			b.BindCommand(info.Command, route[operation], middlewares...)
		}
		if info.CallbackQuery != "" {
			b.BindCallback(info.CallbackQuery, route[operation], middlewares...)
		}
	}
}
