package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"
)

// utf16Len is the reference implementation of UTF-16 length used by the tests.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func makeEntity(offset, length int) models.MessageEntity {
	return models.MessageEntity{Offset: offset, Length: length}
}

func TestUtf16ByteRange(t *testing.T) {
	// "@bot" appears at UTF-16 offset 3 in "你好 @bot" (each CJK rune is one
	// UTF-16 unit but three UTF-8 bytes) and at offset 2 in "🙂@bot" (emoji is a
	// surrogate pair, two UTF-16 units, four UTF-8 bytes).
	tests := []struct {
		name   string
		text   string
		offset int
		length int
		want   string
	}{
		{"ascii", "hello @bot", 6, 4, "@bot"},
		{"cjk prefix", "你好 @bot", 3, 4, "@bot"},
		{"emoji prefix", "🙂@bot", 2, 4, "@bot"},
		{"cjk+emoji", "你好🙂 @bot", 5, 4, "@bot"},
		{"end of string", "prefix@bot", 6, 4, "@bot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := utf16ByteRange(tt.text, tt.offset, tt.length)
			if !ok {
				t.Fatalf("utf16ByteRange(%q, %d, %d) reported out of bounds", tt.text, tt.offset, tt.length)
			}
			if got := tt.text[start:end]; got != tt.want {
				t.Errorf("slice = %q, want %q (byte range [%d,%d))", got, tt.want, start, end)
			}
		})
	}
}

func TestUtf16ByteRangeOutOfBounds(t *testing.T) {
	if _, _, ok := utf16ByteRange("hi", 0, 100); ok {
		t.Error("range beyond the end of the string must be rejected")
	}
	if _, _, ok := utf16ByteRange("hi", 5, 2); ok {
		t.Error("offset beyond the end of the string must be rejected")
	}
	if _, _, ok := utf16ByteRange("hi", -1, 2); ok {
		t.Error("negative offset must be rejected")
	}
	if _, _, ok := utf16ByteRange("hi", 0, -1); ok {
		t.Error("negative length must be rejected")
	}
}

func TestTrimBotMentions(t *testing.T) {
	mentionEntity := func(text, needle string) models.MessageEntity {
		byteOffset := strings.Index(text, needle)
		if byteOffset < 0 {
			t.Fatalf("%q not found in %q", needle, text)
		}
		e := makeEntity(utf16Offset(text, byteOffset), utf16Len(needle))
		e.Type = models.MessageEntityTypeMention
		return e
	}

	tests := []struct {
		name        string
		text        string
		entities    func() []models.MessageEntity
		botID       int64
		username    string
		wantHit     bool
		wantTrimmed *string // expected text after trimming; nil skips the check
	}{
		{
			name: "mention with cjk prefix",
			text: "你好 @mybot 帮我",
			entities: func() []models.MessageEntity {
				return []models.MessageEntity{mentionEntity("你好 @mybot 帮我", "@mybot")}
			},
			username:    "mybot",
			wantHit:     true,
			wantTrimmed: strPtr("你好  帮我"),
		},
		{
			name:        "mention with emoji prefix",
			text:        "🎉@mybot hi",
			entities:    func() []models.MessageEntity { return []models.MessageEntity{mentionEntity("🎉@mybot hi", "@mybot")} },
			username:    "mybot",
			wantHit:     true,
			wantTrimmed: strPtr("🎉 hi"),
		},
		{
			name:     "mention of another bot is not a hit",
			text:     "hello @other",
			entities: func() []models.MessageEntity { return []models.MessageEntity{mentionEntity("hello @other", "@other")} },
			username: "mybot",
			wantHit:  false,
		},
		{
			name: "bot command with suffix",
			text: "/cmd@mybot arg",
			entities: func() []models.MessageEntity {
				e := mentionEntity("/cmd@mybot arg", "/cmd@mybot")
				e.Type = models.MessageEntityTypeBotCommand
				return []models.MessageEntity{e}
			},
			username:    "mybot",
			wantHit:     true,
			wantTrimmed: strPtr("/cmd arg"),
		}, {
			name: "text_mention by id",
			text: "ping",
			entities: func() []models.MessageEntity {
				e := mentionEntity("ping", "ping")
				e.Type = models.MessageEntityTypeTextMention
				e.User = &models.User{ID: 42}
				return []models.MessageEntity{e}
			},
			botID:       42,
			username:    "mybot",
			wantHit:     true,
			wantTrimmed: strPtr(""),
		},
		{
			name: "unrelated entity type is ignored",
			text: "bold text",
			entities: func() []models.MessageEntity {
				e := mentionEntity("bold text", "bold")
				e.Type = models.MessageEntityTypeBold
				return []models.MessageEntity{e}
			},
			username: "mybot",
			wantHit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hit := trimBotMentions(tt.text, tt.entities(), tt.botID, tt.username, true)
			if hit != tt.wantHit {
				t.Fatalf("hit = %v, want %v", hit, tt.wantHit)
			}
			if tt.wantTrimmed != nil && got != *tt.wantTrimmed {
				t.Errorf("trimmed text = %q, want %q", got, *tt.wantTrimmed)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func utf16Offset(text string, byteOffset int) int {
	u16 := 0
	for i, r := range text {
		if i >= byteOffset {
			break
		}
		u16 += utf16.RuneLen(r)
	}
	return u16
}

func TestTrimBotMentionsInvalidEntityDoesNotPanic(t *testing.T) {
	// An entity pointing past the end of the string must be skipped.
	text := "short"
	entities := []models.MessageEntity{makeEntity(0, 100)}
	got, hit := trimBotMentions(text, entities, 1, "bot", true)
	if hit {
		t.Fatal("out-of-bounds entity must not count as a mention")
	}
	if got != text {
		t.Errorf("text changed: %q", got)
	}
}

func TestCallbackSingleFlightKey(t *testing.T) {
	base := &models.CallbackQuery{
		ID:   "cq1",
		Data: "menu:1",
		Message: models.MaybeInaccessibleMessage{
			Message: &models.Message{
				ID: 7,
				Chat: models.Chat{
					ID: -100,
				},
			},
		},
	}
	otherData := *base
	otherData.Data = "menu:2"
	same := *base

	if callbackSingleFlightKey(base) == callbackSingleFlightKey(&otherData) {
		t.Error("different button data on the same message must produce different keys")
	}
	if callbackSingleFlightKey(base) != callbackSingleFlightKey(&same) {
		t.Error("identical callback queries must produce the same key")
	}

	inline := &models.CallbackQuery{ID: "cq2", Data: "menu:1", InlineMessageID: "im1"}
	if !strings.HasPrefix(callbackSingleFlightKey(inline), "inline:im1:") {
		t.Errorf("unexpected inline key: %q", callbackSingleFlightKey(inline))
	}

	deleted := &models.CallbackQuery{ID: "cq3", Data: "menu:1"}
	if !strings.HasPrefix(callbackSingleFlightKey(deleted), "unknown:cq3:") {
		t.Errorf("unexpected fallback key: %q", callbackSingleFlightKey(deleted))
	}
}

func TestNewSingleFlightMiddlewareSameKeyShared(t *testing.T) {
	sf := NewSingleFlightMiddleware()
	var calls int
	var mu sync.Mutex
	var wg sync.WaitGroup

	handler := sf(func(ctx context.Context, update *Update) error {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		return nil
	})

	update := &Update{CallbackQuery: &models.CallbackQuery{
		ID:      "cq",
		Data:    "menu:1",
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 7, Chat: models.Chat{ID: -100}}},
	}}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = handler(context.Background(), update)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("identical concurrent callbacks executed %d times, want 1", calls)
	}
}

func TestNewSingleFlightMiddlewareDifferentDataBothRun(t *testing.T) {
	sf := NewSingleFlightMiddleware()
	var calls int
	var mu sync.Mutex
	var wg sync.WaitGroup

	handler := sf(func(ctx context.Context, update *Update) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})

	base := models.MaybeInaccessibleMessage{Message: &models.Message{ID: 7, Chat: models.Chat{ID: -100}}}
	for _, data := range []string{"menu:inc", "menu:reset"} {
		wg.Add(1)
		go func(data string) {
			defer wg.Done()
			_ = handler(context.Background(), &Update{CallbackQuery: &models.CallbackQuery{ID: "cq", Data: data, Message: base}})
		}(data)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("different button data executed %d times, want 2", calls)
	}
}

func TestSingleFlightErrorShared(t *testing.T) {
	sf := NewSingleFlightMiddleware()
	expected := errors.New("boom")
	handler := sf(func(ctx context.Context, update *Update) error {
		time.Sleep(20 * time.Millisecond)
		return expected
	})

	update := &Update{CallbackQuery: &models.CallbackQuery{
		ID:      "cq",
		Data:    "a",
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 1, Chat: models.Chat{ID: 2}}},
	}}

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = handler(context.Background(), update)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if !errors.Is(err, expected) {
			t.Errorf("shared callbacks must all observe the same error, got %v", err)
		}
	}
}

func TestGroupFilterCaptionDoesNotTouchText(t *testing.T) {
	// Regression for the caption branch overwriting Message.Text: a photo with a
	// caption that mentions the bot must only update the caption.
	var gotText, gotCaption string
	update := &Update{Message: &models.Message{
		Text: "",
		Chat: models.Chat{Type: models.ChatTypeGroup},
		Photo: []models.PhotoSize{
			{FileID: "f", Width: 1, Height: 1},
		},
		Caption: "photo caption @mybot",
		CaptionEntities: []models.MessageEntity{{
			Type:   models.MessageEntityTypeMention,
			Offset: 14,
			Length: 6,
		}},
	}}
	called := false
	filter := NewGroupMessageFilterMiddleware(&fakeGroupBot{id: 1, username: "mybot"}, true, time.Hour)
	err := filter(func(ctx context.Context, update *Update) error {
		called = true
		gotText = update.Message.Text
		gotCaption = update.Message.Caption
		return nil
	})(context.Background(), update)

	if err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if !called {
		t.Fatal("mentioning message was not delivered")
	}
	if gotText != "" {
		t.Errorf("Message.Text was polluted with the caption: %q", gotText)
	}
	if strings.Contains(gotCaption, "@mybot") {
		t.Errorf("mention was not trimmed from caption: %q", gotCaption)
	}
}

type fakeGroupBot struct {
	id       int64
	username string
}

func (f *fakeGroupBot) GetMe(ctx context.Context) (*models.User, error) {
	return &models.User{ID: f.id, Username: f.username}, nil
}

func TestGroupFilterNonMentionGroupMessageDropped(t *testing.T) {
	update := &Update{Message: &models.Message{
		Text:     "普通群消息不触发",
		Chat:     models.Chat{Type: models.ChatTypeSupergroup},
		Entities: []models.MessageEntity{makeEntity(0, 4)}, // unrelated entity e.g. bold
	}}
	filter := NewGroupMessageFilterMiddleware(&fakeGroupBot{id: 1, username: "mybot"}, true, time.Hour)
	called := false
	err := filter(func(ctx context.Context, update *Update) error {
		called = true
		return nil
	})(context.Background(), update)
	if err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if called {
		t.Error("group message that does not mention the bot must be dropped")
	}
}

func TestGroupFilterReplyToBotMessageDelivered(t *testing.T) {
	update := &Update{Message: &models.Message{
		Text: "reply body",
		Chat: models.Chat{Type: models.ChatTypeGroup},
		ReplyToMessage: &models.Message{
			From: &models.User{ID: 1},
		},
	}}
	filter := NewGroupMessageFilterMiddleware(&fakeGroupBot{id: 1, username: "mybot"}, true, time.Hour)
	called := false
	err := filter(func(ctx context.Context, update *Update) error {
		called = true
		return nil
	})(context.Background(), update)
	if err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if !called {
		t.Error("a reply to one of the bot's messages must be delivered")
	}
}

func TestGroupFilterReplyToServiceMessageNoPanic(t *testing.T) {
	update := &Update{Message: &models.Message{
		Text: "reply to service message",
		Chat: models.Chat{Type: models.ChatTypeGroup},
		ReplyToMessage: &models.Message{
			From: nil, // service messages have no sender
		},
	}}
	filter := NewGroupMessageFilterMiddleware(&fakeGroupBot{id: 1, username: "mybot"}, true, time.Hour)
	called := false
	err := filter(func(ctx context.Context, update *Update) error {
		called = true
		return nil
	})(context.Background(), update)
	if err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if called {
		t.Error("reply to a service message must not count as addressing the bot")
	}
}

func TestGroupFilterBotInfoCached(t *testing.T) {
	bot := &fakeGroupBot{id: 1, username: "mybot"}
	var calls int
	wrapped := &countingBot{b: bot, count: &calls}
	filter := NewGroupMessageFilterMiddleware(wrapped, true, time.Hour)
	ctx := context.Background()

	update := func() *Update {
		return &Update{Message: &models.Message{
			Text:     "hi @mybot",
			Chat:     models.Chat{Type: models.ChatTypeGroup},
			Entities: []models.MessageEntity{makeEntity(3, 6)},
		}}
	}
	for i := 0; i < 3; i++ {
		_ = filter(func(ctx context.Context, update *Update) error { return nil })(ctx, update())
	}
	if calls != 1 {
		t.Errorf("GetMe called %d times, want 1 (cache)", calls)
	}
}

type countingBot struct {
	b     *fakeGroupBot
	count *int
}

func (c *countingBot) GetMe(ctx context.Context) (*models.User, error) {
	*c.count++
	return c.b.GetMe(ctx)
}

func TestUtf16LenReference(t *testing.T) {
	cases := map[string]int{
		"hello": 5,
		"你好":    2,
		"🙂":     2,
		"a🙂b":   4,
	}
	for s, want := range cases {
		if got := utf16Len(s); got != want {
			t.Errorf("utf16Len(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestSingleFlightMiddlewareSkipsNonCallback(t *testing.T) {
	sf := NewSingleFlightMiddleware()
	called := false
	handler := sf(func(ctx context.Context, update *Update) error {
		called = true
		return nil
	})
	if err := handler(context.Background(), &Update{Message: &models.Message{Text: "hi"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("regular messages must bypass the singleflight gate")
	}
}
