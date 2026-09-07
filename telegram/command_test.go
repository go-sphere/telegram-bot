package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestMatchCommandMessage(t *testing.T) {
	msg := func(text string) *models.Message { return &models.Message{Text: text} }
	tests := []struct {
		name    string
		message *models.Message
		command string
		want    bool
	}{
		{"exact match", msg("/start"), "start", true},
		{"with args", msg("/start arg1 arg2"), "start", true},
		{"with @bot suffix", msg("/start@MyBot"), "start", true},
		{"with @bot suffix and args", msg("/start@MyBot hello"), "start", true},
		{"command followed by newline", msg("/start\nsecond line"), "start", true},
		{"prefix does not match", msg("/startup"), "start", false},
		{"command in the middle does not match", msg("hello /start"), "start", false},
		{"missing slash", msg("start"), "start", false},
		{"other command", msg("/stop"), "start", false},
		// A command carrying another bot's @suffix still reaches the registry;
		// NewGroupMessageFilterMiddleware is responsible for dropping it in group
		// chats (the bot cannot know its own username without a GetMe call).
		{"cross-bot suffix reaches registry", msg("/help@OtherBot"), "help", true},
		{"nil message", nil, "start", false},
		{"registered command keeps slash", msg("/start"), "/start", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := "<nil>"
			if tt.message != nil {
				text = tt.message.Text
			}
			if got := matchCommandMessage(tt.message, tt.command); got != tt.want {
				t.Errorf("matchCommandMessage(%q, %q) = %v, want %v", text, tt.command, got, tt.want)
			}
		})
	}
}

func TestBindCommandOfflineRouting(t *testing.T) {
	// Full routing integration. bot.WithSkipGetMe avoids the network call and
	// bot.WithNotAsyncHandlers makes ProcessUpdate run synchronously.
	ctx := context.Background()
	app, err := NewApp(
		Config{Token: "123:abc"},
		AppendBotOptions(bot.WithNotAsyncHandlers()),
		WithDeleteWebhookOnStart(false),
	)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	var mu sync.Mutex
	var fired []string
	app.BindCommand("start", func(ctx context.Context, update *Update) error {
		mu.Lock()
		fired = append(fired, update.Message.Text)
		mu.Unlock()
		return nil
	})

	for _, text := range []string{"/start", "/startup", "hello /start", "/start@MyBot", "/stop"} {
		app.bot.ProcessUpdate(ctx, &models.Update{
			ID:      int64(len(fired) + 1),
			Message: &models.Message{Text: text},
		})
	}

	mu.Lock()
	got := append([]string(nil), fired...)
	mu.Unlock()

	want := []string{"/start", "/start@MyBot"}
	if len(got) != len(want) {
		t.Fatalf("fired = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fired = %v, want %v", got, want)
		}
	}
}
