package telegram_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"

	"github.com/go-sphere/telegram-bot/telegram"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/time/rate"
)

// This example routes updates offline: WithNotAsyncHandlers makes
// ProcessUpdate run the matching handler synchronously, so no network or
// polling is needed. A real bot calls Start instead (see Bot.Start).
func Example() {
	app, err := telegram.NewApp(
		telegram.Config{Token: "123:test-token"},
		telegram.AppendBotOptions(bot.WithNotAsyncHandlers()),
	)
	if err != nil {
		fmt.Println("new app:", err)
		return
	}
	app.BindCommand("start", func(ctx context.Context, update *telegram.Update) error {
		uid, _ := telegram.AuthUserID(ctx)
		fmt.Println("start from user", uid)
		return nil
	})
	app.BindNoRoute(func(ctx context.Context, update *telegram.Update) error {
		fmt.Println("no route:", update.Message.Text)
		return nil
	})

	ctx := context.Background()
	for _, text := range []string{"/start", "/startup"} {
		app.API().ProcessUpdate(ctx, &telegram.Update{Message: &models.Message{
			Text: text,
			From: &models.User{ID: 42},
		}})
	}
	// Output:
	// start from user 42
	// no route: /startup
}

// This example shows the full lifecycle of a polling bot. It needs a real
// token and network access, so it is compiled but not run.
func ExampleBot_Start() {
	app, err := telegram.NewApp(telegram.Config{Token: os.Getenv("TELEGRAM_BOT_TOKEN")})
	if err != nil {
		fmt.Println("new app:", err)
		return
	}
	app.BindCommand("start", func(ctx context.Context, update *telegram.Update) error {
		return app.SendMessage(ctx, update, &telegram.Message{Text: "Hello!"})
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.Start(ctx); err != nil { // blocks until ctx is cancelled
		fmt.Println("start:", err)
	}
}

// This example points the client at a local test server to show which Bot API
// method SendMessage uses for a regular message.
func ExampleBot_SendMessage() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		fmt.Println("called", r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"chat":{"id":7}}}`)
	}))
	defer srv.Close()

	app, err := telegram.NewApp(
		telegram.Config{Token: "123:test-token"},
		telegram.AppendBotOptions(bot.WithServerURL(srv.URL)),
	)
	if err != nil {
		fmt.Println("new app:", err)
		return
	}
	update := &telegram.Update{Message: &models.Message{Chat: models.Chat{ID: 7}}}
	if err := app.SendMessage(context.Background(), update, &telegram.Message{Text: "hi"}); err != nil {
		fmt.Println("send:", err)
	}
	// Output: called sendMessage
}

func ExampleNewButton() {
	type page struct {
		N int `json:"n"` // only fields with a json tag are encoded
	}

	btn, err := telegram.NewButton("Next", "page", page{N: 2})
	if err != nil {
		fmt.Println("button:", err)
		return
	}
	// A handler registered with BindCallback("page", ...) receives this data
	// in update.CallbackQuery.Data.
	route, p, err := telegram.UnmarshalData[page](btn.CallbackData)
	if err != nil {
		fmt.Println("decode:", err)
		return
	}
	fmt.Println(btn.Text, route, p.N)
	// Output: Next page 2
}

func ExampleMarshalData_tooLong() {
	_, err := telegram.MarshalData("route", strings.Repeat("x", 80))
	fmt.Println(errors.Is(err, telegram.ErrCallbackDataTooLong))
	// Output: true
}

func ExampleBroadcastMessage() {
	users := []int64{1, 2, 3}
	limiter := rate.NewLimiter(rate.Inf, 1) // use a finite rate for real broadcasts

	err := telegram.BroadcastMessage(context.Background(), nil, users, limiter,
		func(ctx context.Context, b *bot.Bot, chatID int64) error {
			if chatID == 2 {
				return errors.New("blocked by user")
			}
			return nil // a real send function would call b.SendMessage
		},
	)
	var be *telegram.BroadcastError
	if errors.As(err, &be) {
		fmt.Printf("sent %d of %d, first error: %v\n", be.Success, be.Total, be.Err)
	}
	// Output: sent 2 of 3, first error: blocked by user
}

func ExampleRetryOnTooManyRequestsErrorContext() {
	attempts := 0
	err := telegram.RetryOnTooManyRequestsErrorContext(context.Background(), 3, func() error {
		attempts++
		if attempts == 1 {
			return &bot.TooManyRequestsError{Message: "Too Many Requests", RetryAfter: 0}
		}
		return nil
	})
	fmt.Println(attempts, err)
	// Output: 2 <nil>
}
