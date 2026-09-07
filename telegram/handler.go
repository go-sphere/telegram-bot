package telegram

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/sync/singleflight"
)

type (
	// HandlerFunc defines a function type for handling Telegram bot updates.
	// It processes an Update and returns an error if processing fails.
	HandlerFunc = func(ctx context.Context, update *Update) error
	// MiddlewareFunc defines a function type for creating middleware that wraps HandlerFunc.
	// It takes a HandlerFunc and returns a wrapped HandlerFunc with additional functionality.
	MiddlewareFunc = func(next HandlerFunc) HandlerFunc
)

type (
	// ErrorHandlerFunc defines a function type for handling errors that occur during update processing.
	// It receives the error along with the bot instance and update that caused the error.
	ErrorHandlerFunc = func(ctx context.Context, bot *bot.Bot, update *Update, err error)
)

// WithMiddleware wraps a HandlerFunc with middleware chain and error handling.
// It applies middleware in reverse order and converts the result to a bot.HandlerFunc.
// If the wrapped handler returns an error, it calls the provided error handler.
func WithMiddleware(h HandlerFunc, e ErrorHandlerFunc, middleware ...MiddlewareFunc) bot.HandlerFunc {
	handler := h
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler) //nolint:nilaway
	}
	return func(ctx context.Context, bot *bot.Bot, update *models.Update) {
		if err := handler(ctx, update); err != nil {
			if e != nil {
				e(ctx, bot, update, err)
			}
		}
	}
}

// callbackSingleFlightKey builds the deduplication key used for callback queries.
// It identifies the originating message (chat + message id, inline id, or the
// callback id when neither is available) together with the pressed button data,
// so pressing different buttons of the same message is not conflated into one
// request. The message can be nil when it was deleted or is an inline message.
func callbackSingleFlightKey(cq *models.CallbackQuery) string {
	var msgID string
	switch {
	case cq.Message.Message != nil:
		msgID = strconv.FormatInt(cq.Message.Message.Chat.ID, 10) + ":" + strconv.Itoa(cq.Message.Message.ID)
	case cq.InlineMessageID != "":
		msgID = "inline:" + cq.InlineMessageID
	default:
		msgID = "unknown:" + cq.ID
	}
	return msgID + ":" + cq.Data
}

// NewSingleFlightMiddleware creates a middleware that prevents duplicate callback query processing.
// It uses singleflight to ensure that multiple identical callback queries (same message and same
// button data) are processed only once.
func NewSingleFlightMiddleware() MiddlewareFunc {
	sf := &singleflight.Group{}
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, update *Update) error {
			if update.CallbackQuery == nil {
				return next(ctx, update)
			}
			key := callbackSingleFlightKey(update.CallbackQuery)
			_, err, _ := sf.Do(key, func() (any, error) {
				return nil, next(ctx, update)
			})
			return err
		}
	}
}

// NewRecoveryMiddleware creates a middleware that recovers from panics in bot handlers.
// It logs any panic that occurs during update processing and prevents the bot from crashing.
func NewRecoveryMiddleware() bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, bot *bot.Bot, update *models.Update) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in bot handler", slog.Any("error", r))
				}
			}()
			next(ctx, bot, update)
		}
	}
}

// utf16ByteRange converts an entity range into byte bounds of s. The Telegram
// Bot API measures MessageEntity offset and length in UTF-16 code units, so they
// cannot be used to slice a Go string directly. ok is false when the range does
// not start and end on rune boundaries within s (for example when the offset
// falls inside a surrogate pair, which never happens for well-formed entities).
func utf16ByteRange(s string, offset, length int) (start, end int, ok bool) {
	if offset < 0 || length < 0 {
		return 0, 0, false
	}
	limit := offset + length
	start, end = -1, -1
	u16 := 0
	for i, r := range s {
		if u16 == offset {
			start = i
		}
		if u16 == limit {
			end = i
			break
		}
		u16 += utf16.RuneLen(r)
	}
	// The entity may end exactly at the end of the string.
	if start >= 0 && end < 0 && u16 == limit {
		end = len(s)
	}
	if start < 0 || end < 0 {
		return 0, 0, false
	}
	return start, end, true
}

// entitySlice returns the substring of s covered by the entity, converting the
// entity's UTF-16 offsets to byte offsets first. ok is false when the range is
// out of bounds.
func entitySlice(s string, e models.MessageEntity) (string, bool) {
	start, end, ok := utf16ByteRange(s, e.Offset, e.Length)
	if !ok {
		return "", false
	}
	return s[start:end], true
}

// removeEntityRange removes the whole range covered by the entity from text.
func removeEntityRange(text string, e models.MessageEntity) string {
	start, end, ok := utf16ByteRange(text, e.Offset, e.Length)
	if !ok {
		return text
	}
	return text[:start] + text[end:]
}

// removeEntitySuffix removes the trailing suffix (for example "@botname") from
// the text covered by the entity, keeping the rest of the entity (for example
// the "/command" itself).
func removeEntitySuffix(text string, e models.MessageEntity, suffix string) string {
	start, end, ok := utf16ByteRange(text, e.Offset, e.Length)
	if !ok {
		return text
	}
	entityText := text[start:end]
	return text[:start] + strings.TrimSuffix(entityText, suffix) + text[end:]
}

// trimBotMentions removes mentions of the bot from text and reports whether the
// text addresses the bot. When trim is false it only detects. Detection covers
// "@username" mentions, text_mention entities pointing at the bot by user id and
// bot commands carrying an "@username" suffix. Trimming stops after the first
// match: entity offsets describe the original text, so removing one mention makes
// every later offset stale and they must not be applied anymore.
func trimBotMentions(text string, entities []models.MessageEntity, botID int64, username string, trim bool) (string, bool) {
	for _, e := range entities {
		entityText, ok := entitySlice(text, e)
		if !ok {
			continue
		}
		switch e.Type {
		case models.MessageEntityTypeMention:
			if entityText == "@"+username {
				if !trim {
					return text, true
				}
				return removeEntityRange(text, e), true
			}
		case models.MessageEntityTypeTextMention:
			if e.User != nil && e.User.ID == botID {
				if !trim {
					return text, true
				}
				return removeEntityRange(text, e), true
			}
		case models.MessageEntityTypeBotCommand:
			if strings.HasSuffix(entityText, "@"+username) {
				if !trim {
					return text, true
				}
				return removeEntitySuffix(text, e, "@"+username), true
			}
		default:
			continue
		}
	}
	return text, false
}

// botInfoFetcher is satisfied by *bot.Bot. It exists so the group filter can be
// tested with a fake without a live bot client.
type botInfoFetcher interface {
	GetMe(ctx context.Context) (*models.User, error)
}

// NewGroupMessageFilterMiddleware creates a middleware that filters group messages based on bot mentions.
// It only processes group messages where the bot is explicitly mentioned through @username, replies,
// or text mentions. The middleware caches bot information to reduce API calls and optionally
// removes mention text from the message content.
//
// Parameters:
//   - b: The bot client used to retrieve bot information
//   - trimMention: Whether to remove mention text from processed messages
//   - infoExpire: Duration to cache bot information before refreshing
func NewGroupMessageFilterMiddleware(b botInfoFetcher, trimMention bool, infoExpire time.Duration) MiddlewareFunc {
	var (
		ts   time.Time
		sf   singleflight.Group
		user *models.User
	)

	// Channel messages reach the bot as channel_post updates, not as
	// update.Message, so channel does not need to be handled here.
	isGroupChatType := func(t models.ChatType) bool {
		return t == models.ChatTypeGroup || t == models.ChatTypeSupergroup
	}

	getBotInfo := func(ctx context.Context, sf *singleflight.Group) (int64, string, error) {
		v, err, _ := sf.Do("getMe", func() (any, error) {
			// Use the cached bot info while it is still fresh.
			if user != nil && time.Since(ts) < infoExpire {
				return user, nil
			}
			u, err := b.GetMe(ctx)
			if err != nil {
				return nil, err
			}
			user = u
			ts = time.Now()
			return u, nil
		})
		if err != nil {
			return 0, "", err
		}
		return v.(*models.User).ID, v.(*models.User).Username, nil
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, update *Update) error {
			// Non-group messages are always processed.
			if update.Message == nil || !isGroupChatType(update.Message.Chat.Type) {
				return next(ctx, update)
			}

			id, username, err := getBotInfo(ctx, &sf)
			if err != nil {
				slog.Error("get bot info error", slog.String("error", err.Error()))
				return err
			}

			// A reply to one of the bot's own messages always addresses the bot.
			if update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.From != nil && update.Message.ReplyToMessage.From.ID == id {
				return next(ctx, update)
			}

			isMention := false

			// Check the text for a mention of the bot.
			if len(update.Message.Entities) > 0 && update.Message.Text != "" {
				text, mention := trimBotMentions(update.Message.Text, update.Message.Entities, id, username, trimMention)
				update.Message.Text = text
				isMention = mention || isMention
			}

			// Check the caption of a media message for a mention of the bot.
			if !isMention && len(update.Message.CaptionEntities) > 0 && update.Message.Caption != "" {
				caption, mention := trimBotMentions(update.Message.Caption, update.Message.CaptionEntities, id, username, trimMention)
				update.Message.Caption = caption
				isMention = mention || isMention
			}

			// Only process group messages that mention the bot.
			if !isMention {
				return nil
			}
			return next(ctx, update)
		}
	}
}
