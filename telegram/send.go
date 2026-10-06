package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/time/rate"
)

// apiClient is the subset of *bot.Bot used by the send helpers. Keeping the
// operations behind an interface makes the dispatch logic unit-testable.
type apiClient interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendPhoto(ctx context.Context, params *bot.SendPhotoParams) (*models.Message, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
	EditMessageCaption(ctx context.Context, params *bot.EditMessageCaptionParams) (*models.Message, error)
	EditMessageMedia(ctx context.Context, params *bot.EditMessageMediaParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
}

var _ apiClient = (*bot.Bot)(nil)

// SendMessage sends or edits a message based on the update type and content.
// For callback queries it edits the original message (when it is still
// accessible), for regular messages it sends a new message. The function
// automatically chooses between text, caption and media edits depending on the
// new content and the original message type.
//
// In detail:
//   - Regular message: sendMessage, or sendPhoto when m.Media is set, to the
//     same chat.
//   - Callback query on an accessible message: editMessageMedia when m.Media
//     is set, editMessageCaption when the original has media, otherwise
//     editMessageText.
//   - Callback query on an inline message: editMessageMedia or
//     editMessageText by inline message id.
//   - Callback query whose message is no longer accessible: a callback answer
//     telling the user the message is unavailable.
//
// It returns nil without a request when update or m is nil, or when the update
// is neither a message nor a callback query. API errors are returned as
// produced by github.com/go-telegram/bot. ctx bounds the HTTP request.
func SendMessage(ctx context.Context, b *bot.Bot, update *Update, m *Message) error {
	return sendMessage(ctx, b, update, m)
}

func sendMessage(ctx context.Context, c apiClient, update *Update, m *Message) error {
	if m == nil || update == nil {
		return nil
	}
	if update.CallbackQuery != nil {
		cq := update.CallbackQuery
		switch {
		case cq.Message.Message != nil:
			return sendEditMessage(ctx, c, cq.Message.Message, m)
		case cq.InlineMessageID != "":
			return sendEditInlineMessage(ctx, c, cq.InlineMessageID, m)
		default:
			// The original message was deleted (only an inaccessible stub is
			// left) and cannot be edited; tell the user instead of panicking.
			_, err := c.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: cq.ID,
				Text:            "This message is no longer available, please send a new request.",
			})
			return err
		}
	}
	if update.Message != nil {
		if m.Media == nil {
			param := m.toSendMessageParams(update.Message.Chat.ID)
			_, err := c.SendMessage(ctx, param)
			return err
		}
		param := m.toSendPhotoParams(update.Message.Chat.ID)
		_, err := c.SendPhoto(ctx, param)
		return err
	}
	return nil
}

// sendEditMessage edits an accessible message. A message carrying new media is
// edited with EditMessageMedia even when the original message is plain text
// (Telegram upgrades it). Otherwise a media message is edited through its
// caption and a text message through its text.
func sendEditMessage(ctx context.Context, c apiClient, origin *models.Message, m *Message) error {
	chatID := origin.Chat.ID
	switch {
	case m.Media != nil:
		param := toEditMessageMediaParams(chatID, origin.ID, m)
		_, err := c.EditMessageMedia(ctx, param)
		return err
	case hasMedia(origin):
		param := m.toEditMessageCaptionParams(chatID, origin.ID)
		_, err := c.EditMessageCaption(ctx, param)
		return err
	default:
		param := m.toEditMessageTextParams(chatID, origin.ID)
		_, err := c.EditMessageText(ctx, param)
		return err
	}
}

// sendEditInlineMessage edits an inline message, identified only by its inline
// message id, by media or by text.
func sendEditInlineMessage(ctx context.Context, c apiClient, inlineMessageID string, m *Message) error {
	if m.Media != nil {
		params := &bot.EditMessageMediaParams{
			InlineMessageID: inlineMessageID,
			ReplyMarkup:     m.replyMarkup(),
			Media:           inputMediaPhoto(m),
		}
		_, err := c.EditMessageMedia(ctx, params)
		return err
	}
	params := &bot.EditMessageTextParams{
		InlineMessageID: inlineMessageID,
		Text:            m.Text,
		ParseMode:       m.ParseMode,
		ReplyMarkup:     m.replyMarkup(),
	}
	_, err := c.EditMessageText(ctx, params)
	return err
}

// SendErrorMessage sends an error message to the user based on the update type.
// For regular messages, it sends a new message with the error text.
// For callback queries, it shows the error in a popup using AnswerCallbackQuery,
// truncated to 200 characters. It does nothing when err or update is nil, and
// ignores errors from the Bot API. Its signature matches ErrorHandlerFunc, so
// it can be passed to WithErrorHandler to show handler errors to users.
func SendErrorMessage(ctx context.Context, b *bot.Bot, update *Update, err error) {
	sendErrorMessage(ctx, b, update, err)
}

func sendErrorMessage(ctx context.Context, c apiClient, update *Update, err error) {
	if err == nil || update == nil {
		return
	}
	if update.Message != nil {
		_, _ = c.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   err.Error(),
		})
	}
	if update.CallbackQuery != nil {
		_, _ = c.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            truncateForToast(err.Error()),
		})
	}
}

// truncateForToast limits the callback toast text to 200 characters, the limit
// Telegram enforces for AnswerCallbackQuery text.
func truncateForToast(s string) string {
	const maxRunes = 200
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// broadcastOptions holds configuration for message broadcasting operations.
type broadcastOptions struct {
	progress            func(int, int, int) // Progress callback: (current, errors, total)
	terminalOnSendError bool                // Whether to stop on first send error
}

// BroadcastOption defines a function type for configuring broadcast operations.
// Pass options to BroadcastMessage.
type BroadcastOption func(*broadcastOptions)

func newBroadcastOptions(opts ...BroadcastOption) *broadcastOptions {
	defaults := &broadcastOptions{
		progress:            nil,
		terminalOnSendError: false,
	}
	for _, opt := range opts {
		opt(defaults)
	}
	return defaults
}

// WithProgress sets a progress callback function for broadcast operations.
// The callback receives (current index, error count, total count) during broadcasting.
// It is called before each item and once more with (total, errors, total)
// after the last item when the broadcast is not stopped early. It runs on the
// goroutine that called BroadcastMessage.
func WithProgress(progress func(int, int, int)) BroadcastOption {
	return func(o *broadcastOptions) {
		o.progress = progress
	}
}

// WithTerminalOnSendError configures whether broadcasting should stop on the first send error.
// If true, broadcasting terminates immediately on any send failure and
// BroadcastMessage returns that send error unwrapped. The default is false.
func WithTerminalOnSendError(terminalOnSendError bool) BroadcastOption {
	return func(o *broadcastOptions) {
		o.terminalOnSendError = terminalOnSendError
	}
}

// BroadcastError reports a broadcast that finished with some failed sends.
// BroadcastMessage returns it as *BroadcastError; use errors.As to read the
// counts. errors.Is and errors.As also see the first send error through Unwrap.
type BroadcastError struct {
	Total   int   // Number of items in the broadcast
	Failed  int   // Number of items whose send function returned an error
	Success int   // Total minus Failed
	Err     error // The first send error encountered
}

// Error reports the success and failure counts and the first send error.
func (e *BroadcastError) Error() string {
	return fmt.Sprintf("broadcast: %d/%d messages sent, %d failed: %v", e.Success, e.Total, e.Failed, e.Err)
}

// Unwrap returns the first send error.
func (e *BroadcastError) Unwrap() error {
	return e.Err
}

// BroadcastMessage sends messages to multiple recipients with rate limiting and error handling.
// It processes each item in the data slice through the provided send function, respecting
// the rate limiter and reporting progress through optional callbacks. Unless
// WithTerminalOnSendError is set, every item is attempted and a *BroadcastError
// summarizing the failures is returned when any send fails.
//
// Items are sent one at a time on the calling goroutine, in slice order. If
// ctx is cancelled, or rateLimiter.Wait fails (for example because a deadline
// would be exceeded), BroadcastMessage stops and returns that error; items
// already sent are not reported. b is passed to send unchanged and may be nil
// when send does not use it.
//
// Type parameter T represents the data type for each broadcast target.
//
// Parameters:
//   - ctx: Context for cancellation
//   - b: Bot instance for sending messages
//   - data: Slice of data items to process
//   - rateLimiter: Rate limiter to control send frequency; must not be nil
//   - send: Function to send message for each data item
//   - options: Optional configuration for progress tracking and error handling
func BroadcastMessage[T any](ctx context.Context, b *bot.Bot, data []T, rateLimiter *rate.Limiter, send func(context.Context, *bot.Bot, T) error, options ...BroadcastOption) error {
	if rateLimiter == nil {
		return errors.New("telegram: rate limiter must not be nil")
	}
	opts := newBroadcastOptions(options...)
	total := len(data)
	var errCount int
	var firstErr error
	for i, d := range data {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opts.progress != nil {
			opts.progress(i, errCount, total)
		}
		if err := rateLimiter.Wait(ctx); err != nil {
			return err
		}
		if err := send(ctx, b, d); err != nil {
			errCount++
			if firstErr == nil {
				firstErr = err
			}
			if opts.terminalOnSendError {
				return err
			}
		}
	}
	if opts.progress != nil {
		opts.progress(total, errCount, total)
	}
	if errCount > 0 {
		return &BroadcastError{
			Total:   total,
			Failed:  errCount,
			Success: total - errCount,
			Err:     firstErr,
		}
	}
	return nil
}

// maxTelegramRetryAfter caps how long a single retry may sleep after a 429
// response, protecting against an abnormally large retry_after value.
const maxTelegramRetryAfter = time.Minute

// RetryOnTooManyRequestsErrorContext implements automatic retry logic for
// Telegram rate limit errors. It respects the RetryAfter duration from
// Telegram's error response (capped at one minute) and retries until ctx is
// cancelled or maxRetries sends have failed. Returns ctx.Err() when the context
// is cancelled while waiting, or an error if max retries are exceeded or a
// non-rate-limit error occurs.
//
// send is called at most maxRetries+1 times. Only errors matching
// *github.com/go-telegram/bot.TooManyRequestsError (via errors.As) are retried;
// any other error is returned unchanged. A negative maxRetries returns an
// error without calling send. The max-retries error does not wrap the last
// rate-limit error.
func RetryOnTooManyRequestsErrorContext(ctx context.Context, maxRetries int, send func() error) error {
	if maxRetries < 0 {
		return errors.New("telegram: max retries exceeded")
	}
	for attempt := 0; ; attempt++ {
		err := send()
		if err == nil {
			return nil
		}
		var tooManyRequestsError *bot.TooManyRequestsError
		if !errors.As(err, &tooManyRequestsError) {
			return err
		}
		if attempt >= maxRetries {
			return errors.New("telegram: max retries exceeded")
		}
		wait := time.Duration(tooManyRequestsError.RetryAfter) * time.Second
		if wait > maxTelegramRetryAfter {
			wait = maxTelegramRetryAfter
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// RetryOnTooManyRequestsError implements automatic retry logic for Telegram rate
// limit errors. It is a convenience wrapper around
// RetryOnTooManyRequestsErrorContext with a background context, so waiting
// cannot be cancelled; prefer the Context variant inside handlers.
func RetryOnTooManyRequestsError(maxRetries int, send func() error) error {
	return RetryOnTooManyRequestsErrorContext(context.Background(), maxRetries, send)
}
