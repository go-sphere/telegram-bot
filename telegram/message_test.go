package telegram

import (
	"bytes"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestMessageParamBuilders(t *testing.T) {
	msg := &Message{
		Text:      "hello <b>world</b>",
		ParseMode: models.ParseModeHTML,
		Media:     NewBytesInputFile("photo.jpg", []byte("img")),
		Button: [][]models.InlineKeyboardButton{
			{{Text: "Go", CallbackData: "go:1"}},
		},
	}

	t.Run("toSendMessageParams", func(t *testing.T) {
		p := msg.toSendMessageParams(123)
		if p.ChatID != int64(123) || p.Text != msg.Text || p.ParseMode != msg.ParseMode {
			t.Errorf("unexpected send params: %+v", p)
		}
		if p.ReplyMarkup == nil {
			t.Fatal("reply markup missing")
		}
		if _, ok := p.ReplyMarkup.(*models.InlineKeyboardMarkup); !ok {
			t.Errorf("unexpected markup type: %T", p.ReplyMarkup)
		}
	})

	t.Run("toSendPhotoParams", func(t *testing.T) {
		p := msg.toSendPhotoParams(123)
		if p.Photo == nil || p.Caption != msg.Text {
			t.Errorf("unexpected photo params: %+v", p)
		}
	})

	t.Run("toEditMessageTextParams", func(t *testing.T) {
		p := msg.toEditMessageTextParams(123, 9)
		if p.MessageID != 9 || p.Text != msg.Text {
			t.Errorf("unexpected edit text params: %+v", p)
		}
	})

	t.Run("toEditMessageCaptionParams", func(t *testing.T) {
		p := msg.toEditMessageCaptionParams(123, 9)
		if p.MessageID != 9 || p.Caption != msg.Text {
			t.Errorf("unexpected edit caption params: %+v", p)
		}
	})

	t.Run("no buttons yields no markup", func(t *testing.T) {
		plain := &Message{Text: "hi"}
		if p := plain.toSendMessageParams(1); p.ReplyMarkup != nil {
			t.Error("no buttons must produce nil reply markup")
		}
	})
}

func TestInputMediaPhoto(t *testing.T) {
	t.Run("upload attaches bytes", func(t *testing.T) {
		data := []byte("fake-image")
		m := &Message{Media: NewBytesInputFile("a.jpg", data)}
		p := inputMediaPhoto(m)
		if p.Media != "attach://a.jpg" {
			t.Errorf("upload media = %q, want attach://a.jpg", p.Media)
		}
		if p.MediaAttachment == nil {
			t.Fatal("upload bytes not attached")
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(p.MediaAttachment)
		if buf.String() != string(data) {
			t.Errorf("attached bytes = %q, want %q", buf.String(), data)
		}
	})

	t.Run("string input passes url through", func(t *testing.T) {
		m := &Message{Media: NewStringInputFile("https://example.com/x.jpg")}
		p := inputMediaPhoto(m)
		if p.Media != "https://example.com/x.jpg" {
			t.Errorf("string media = %q", p.Media)
		}
		if p.MediaAttachment != nil {
			t.Error("string input must not attach bytes")
		}
	})

	t.Run("no media", func(t *testing.T) {
		p := inputMediaPhoto(&Message{})
		if p.Media != "" {
			t.Errorf("expected empty media, got %q", p.Media)
		}
	})

	t.Run("caption and parse mode forwarded", func(t *testing.T) {
		m := &Message{Text: "cap", ParseMode: models.ParseModeHTML}
		p := inputMediaPhoto(m)
		if p.Caption != "cap" || p.ParseMode != models.ParseModeHTML {
			t.Errorf("caption/parse mode not forwarded: %+v", p)
		}
	})
}

func TestToEditMessageMediaParams(t *testing.T) {
	m := &Message{Text: "caption", Media: NewStringInputFile("fid"), Button: [][]models.InlineKeyboardButton{{{Text: "b"}}}}
	p := toEditMessageMediaParams(int64(1), 5, m)
	if p.ChatID != int64(1) || p.MessageID != 5 {
		t.Errorf("chat/message id wrong: %+v", p)
	}
	media, ok := p.Media.(*models.InputMediaPhoto)
	if !ok {
		t.Fatalf("expected InputMediaPhoto, got %T", p.Media)
	}
	if media.Media != "fid" {
		t.Errorf("media = %q", media.Media)
	}
	if p.ReplyMarkup == nil {
		t.Error("reply markup must be carried to the edit")
	}
}

func TestHasMedia(t *testing.T) {
	text := &models.Message{Text: "hi"}
	if hasMedia(text) {
		t.Error("plain text message must not count as media")
	}
	photo := &models.Message{Photo: []models.PhotoSize{{FileID: "x"}}}
	if !hasMedia(photo) {
		t.Error("photo message must count as media")
	}
	video := &models.Message{Video: &models.Video{}}
	if !hasMedia(video) {
		t.Error("video message must count as media")
	}
}
