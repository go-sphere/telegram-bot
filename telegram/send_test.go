package telegram

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/time/rate"
)

// recordingClient records the API calls made through the send helpers so tests
// can assert the dispatch decisions without touching the network.
type recordingClient struct {
	calls []string

	sendErr error
	editErr error
}

func (r *recordingClient) record(call string) {
	r.calls = append(r.calls, call)
}

func (r *recordingClient) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	r.record("SendMessage")
	if r.sendErr != nil {
		return nil, r.sendErr
	}
	return &models.Message{}, nil
}

func (r *recordingClient) SendPhoto(ctx context.Context, params *bot.SendPhotoParams) (*models.Message, error) {
	r.record("SendPhoto")
	if r.sendErr != nil {
		return nil, r.sendErr
	}
	return &models.Message{}, nil
}

func (r *recordingClient) EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error) {
	r.record("EditMessageText")
	if r.editErr != nil {
		return nil, r.editErr
	}
	return &models.Message{}, nil
}

func (r *recordingClient) EditMessageCaption(ctx context.Context, params *bot.EditMessageCaptionParams) (*models.Message, error) {
	r.record("EditMessageCaption")
	if r.editErr != nil {
		return nil, r.editErr
	}
	return &models.Message{}, nil
}

func (r *recordingClient) EditMessageMedia(ctx context.Context, params *bot.EditMessageMediaParams) (*models.Message, error) {
	r.record("EditMessageMedia")
	if r.editErr != nil {
		return nil, r.editErr
	}
	return &models.Message{}, nil
}

func (r *recordingClient) AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error) {
	r.record("AnswerCallbackQuery")
	return true, nil
}

func callbackUpdate(message *models.Message) *Update {
	return &Update{CallbackQuery: &models.CallbackQuery{
		ID:      "cq1",
		Message: models.MaybeInaccessibleMessage{Message: message},
	}}
}

func textMessage() *models.Message {
	return &models.Message{ID: 7, Chat: models.Chat{ID: -100}}
}

func photoMessage() *models.Message {
	return &models.Message{ID: 7, Chat: models.Chat{ID: -100}, Photo: []models.PhotoSize{{FileID: "f"}}}
}

func TestSendMessageRegularMessage(t *testing.T) {
	c := &recordingClient{}
	tests := []struct {
		name string
		msg  *Message
		want string
	}{
		{"text", &Message{Text: "hi"}, "SendMessage"},
		{"photo", &Message{Text: "cap", Media: NewStringInputFile("fid")}, "SendPhoto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := &Update{Message: textMessage()}
			if err := sendMessage(context.Background(), c, upd, tt.msg); err != nil {
				t.Fatalf("sendMessage: %v", err)
			}
			if len(c.calls) != 1 || c.calls[0] != tt.want {
				t.Fatalf("calls = %v, want [%s]", c.calls, tt.want)
			}
			c.calls = nil
		})
	}
}

func TestSendMessageCallbackDispatch(t *testing.T) {
	tests := []struct {
		name     string
		orig     *models.Message
		msg      *Message
		wantCall string
	}{
		{
			name:     "edit text of a text message",
			orig:     textMessage(),
			msg:      &Message{Text: "new"},
			wantCall: "EditMessageText",
		},
		{
			name:     "edit caption of a media message",
			orig:     photoMessage(),
			msg:      &Message{Text: "new cap"},
			wantCall: "EditMessageCaption",
		},
		{
			name:     "edit media of a media message",
			orig:     photoMessage(),
			msg:      &Message{Media: NewStringInputFile("other")},
			wantCall: "EditMessageMedia",
		},
		{
			name:     "upgrade text message to media via edit media",
			orig:     textMessage(),
			msg:      &Message{Media: NewStringInputFile("other")},
			wantCall: "EditMessageMedia",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &recordingClient{}
			if err := sendMessage(context.Background(), c, callbackUpdate(tt.orig), tt.msg); err != nil {
				t.Fatalf("sendMessage: %v", err)
			}
			if len(c.calls) != 1 || c.calls[0] != tt.wantCall {
				t.Fatalf("calls = %v, want [%s]", c.calls, tt.wantCall)
			}
		})
	}
}

func TestSendMessageCallbackInaccessibleMessage(t *testing.T) {
	// A callback whose message was deleted has only the inaccessible stub; the
	// sender must show a toast instead of panicking.
	c := &recordingClient{}
	upd := &Update{CallbackQuery: &models.CallbackQuery{
		ID:      "cq-deleted",
		Message: models.MaybeInaccessibleMessage{InaccessibleMessage: &models.InaccessibleMessage{}},
	}}
	if err := sendMessage(context.Background(), c, upd, &Message{Text: "hi"}); err != nil {
		t.Fatalf("sendMessage on deleted message: %v", err)
	}
	if len(c.calls) != 1 || c.calls[0] != "AnswerCallbackQuery" {
		t.Fatalf("calls = %v, want [AnswerCallbackQuery]", c.calls)
	}
}

func TestSendMessageCallbackInlineMessage(t *testing.T) {
	c := &recordingClient{}
	upd := &Update{CallbackQuery: &models.CallbackQuery{
		ID:              "cq-inline",
		InlineMessageID: "im1",
	}}
	if err := sendMessage(context.Background(), c, upd, &Message{Text: "edit"}); err != nil {
		t.Fatalf("sendMessage inline: %v", err)
	}
	if len(c.calls) != 1 || c.calls[0] != "EditMessageText" {
		t.Fatalf("calls = %v, want [EditMessageText]", c.calls)
	}
}

func TestSendMessageNilGuard(t *testing.T) {
	c := &recordingClient{}
	if err := sendMessage(context.Background(), c, nil, &Message{Text: "hi"}); err != nil {
		t.Fatalf("nil update: %v", err)
	}
	if err := sendMessage(context.Background(), c, &Update{}, nil); err != nil {
		t.Fatalf("nil message: %v", err)
	}
	if len(c.calls) != 0 {
		t.Fatalf("no API call expected, got %v", c.calls)
	}
}

func TestSendMessageErrorPropagated(t *testing.T) {
	sentinel := errors.New("telegram is down")
	c := &recordingClient{sendErr: sentinel}
	err := sendMessage(context.Background(), c, &Update{Message: textMessage()}, &Message{Text: "hi"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}

func TestSendErrorMessage(t *testing.T) {
	c := &recordingClient{}
	err := errors.New("something failed")
	upd := &Update{
		Message:       textMessage(),
		CallbackQuery: &models.CallbackQuery{ID: "cq"},
	}
	sendErrorMessage(context.Background(), c, upd, err)
	want := []string{"SendMessage", "AnswerCallbackQuery"}
	if len(c.calls) != 2 || c.calls[0] != want[0] || c.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", c.calls, want)
	}
}

func TestSendErrorMessageNil(t *testing.T) {
	c := &recordingClient{}
	sendErrorMessage(context.Background(), c, &Update{Message: textMessage()}, nil)
	if len(c.calls) != 0 {
		t.Fatalf("nil error must not send anything, got %v", c.calls)
	}
}

func TestTruncateForToast(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := truncateForToast(long)
	if len([]rune(got)) != 200 {
		t.Errorf("truncated length = %d, want 200", len([]rune(got)))
	}
	if got := truncateForToast("short"); got != "short" {
		t.Errorf("short text must stay untouched, got %q", got)
	}
}

func TestBroadcastMessage(t *testing.T) {
	noop := func(context.Context, *bot.Bot, int) error { return nil }
	t.Run("all success returns nil", func(t *testing.T) {
		err := BroadcastMessage(context.Background(), &bot.Bot{}, []int{1, 2, 3}, rate.NewLimiter(rate.Limit(1000), 10), noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("partial failure returns aggregate error", func(t *testing.T) {
		send := func(ctx context.Context, b *bot.Bot, i int) error {
			if i == 2 {
				return errors.New("boom")
			}
			return nil
		}
		err := BroadcastMessage(context.Background(), &bot.Bot{}, []int{1, 2, 3}, rate.NewLimiter(rate.Limit(1000), 10), send)
		var be *BroadcastError
		if !errors.As(err, &be) {
			t.Fatalf("expected *BroadcastError, got %v", err)
		}
		if be.Total != 3 || be.Success != 2 || be.Failed != 1 {
			t.Errorf("counts = total %d success %d failed %d", be.Total, be.Success, be.Failed)
		}
	})

	t.Run("terminal on error stops early", func(t *testing.T) {
		var attempts int32
		send := func(ctx context.Context, b *bot.Bot, i int) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("stop")
		}
		err := BroadcastMessage(context.Background(), &bot.Bot{}, []int{1, 2, 3}, rate.NewLimiter(rate.Limit(1000), 10), send, WithTerminalOnSendError(true))
		if err == nil {
			t.Fatal("expected terminal error")
		}
		if atomic.LoadInt32(&attempts) != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("nil limiter returns error", func(t *testing.T) {
		err := BroadcastMessage(context.Background(), &bot.Bot{}, []int{1}, nil, noop)
		if err == nil {
			t.Fatal("nil limiter must return an error")
		}
	})

	t.Run("context cancellation aborts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		send := func(ctx context.Context, b *bot.Bot, i int) error {
			cancel() // cancel before the second send
			return nil
		}
		slow := rate.NewLimiter(rate.Every(time.Hour), 1) // second Wait blocks forever
		err := BroadcastMessage(ctx, &bot.Bot{}, []int{1, 2}, slow, send)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestRetryOnTooManyRequestsErrorContext(t *testing.T) {
	sentinel := errors.New("permanent")

	t.Run("no error on first try", func(t *testing.T) {
		var attempts int
		err := RetryOnTooManyRequestsErrorContext(context.Background(), 3, func() error {
			attempts++
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("non rate limit error is not retried", func(t *testing.T) {
		var attempts int
		err := RetryOnTooManyRequestsErrorContext(context.Background(), 3, func() error {
			attempts++
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want %v", err, sentinel)
		}
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("recovers after retry_after waits", func(t *testing.T) {
		var attempts int
		err := RetryOnTooManyRequestsErrorContext(context.Background(), 3, func() error {
			attempts++
			if attempts == 1 {
				return &bot.TooManyRequestsError{RetryAfter: 0}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if attempts != 2 {
			t.Errorf("attempts = %d, want 2", attempts)
		}
	})

	t.Run("max retries exhausted", func(t *testing.T) {
		var attempts int
		err := RetryOnTooManyRequestsErrorContext(context.Background(), 2, func() error {
			attempts++
			return &bot.TooManyRequestsError{RetryAfter: 0}
		})
		if err == nil {
			t.Fatal("expected error after exhausting retries")
		}
		if attempts != 3 {
			t.Errorf("attempts = %d, want 3 (initial + 2 retries)", attempts)
		}
	})

	t.Run("context cancellation aborts wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var attempts int
		_ = RetryOnTooManyRequestsErrorContext(ctx, 5, func() error {
			attempts++
			go cancel() // cancel while waiting on the timer
			return &bot.TooManyRequestsError{RetryAfter: 60}
		})
		if attempts != 1 {
			t.Errorf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("negative max retries rejected", func(t *testing.T) {
		err := RetryOnTooManyRequestsErrorContext(context.Background(), -1, func() error { return nil })
		if err == nil {
			t.Fatal("expected error for negative max retries")
		}
	})

	t.Run("legacy wrapper still works", func(t *testing.T) {
		var attempts int
		err := RetryOnTooManyRequestsError(1, func() error {
			attempts++
			if attempts == 1 {
				return &bot.TooManyRequestsError{RetryAfter: 0}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
