package telegram

import (
	"bytes"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Button is an alias for Telegram's inline keyboard button.
type Button = models.InlineKeyboardButton

// NewButton creates an inline keyboard button with text, callback route, and data.
// The route and data are marshaled together to form the callback data. The
// returned error is non-nil when the route is empty or contains ":", or when the
// marshaled payload exceeds the 64 bytes Telegram allows for callback data.
func NewButton[T any](text, route string, data T) (Button, error) {
	callbackData, err := MarshalData(route, data)
	if err != nil {
		return Button{}, err
	}
	return Button{
		Text:         text,
		CallbackData: callbackData,
	}, nil
}

// NewURLButton creates an inline keyboard button that opens a URL when pressed.
func NewURLButton(text, url string) Button {
	return Button{
		Text: text,
		URL:  url,
	}
}

// NewBytesInputFile creates an InputFile from a byte slice for file uploads.
// The name parameter specifies the filename that will be used in Telegram.
func NewBytesInputFile(name string, data []byte) models.InputFile {
	return &models.InputFileUpload{
		Filename: name,
		Data:     bytes.NewReader(data),
	}
}

// NewStringInputFile creates an InputFile from a URL string for media sharing.
// This is used when referencing existing media by URL or file ID.
func NewStringInputFile(url string) models.InputFile {
	return &models.InputFileString{
		Data: url,
	}
}

// Message represents a complete message that can be sent or edited in Telegram.
// It supports text content, media attachments, formatting, and inline keyboards.
type Message struct {
	Text      string                          // Message text content
	Media     models.InputFile                // Optional media attachment (photo, document, etc.)
	ParseMode models.ParseMode                // Text parsing mode (HTML, Markdown, etc.)
	Button    [][]models.InlineKeyboardButton // Inline keyboard layout as rows of buttons
}

func (m *Message) replyMarkup() models.ReplyMarkup {
	if len(m.Button) > 0 {
		return &models.InlineKeyboardMarkup{
			InlineKeyboard: m.Button,
		}
	}
	return nil
}

// inputMediaPhoto builds an InputMediaPhoto from the message media. The upload
// variant attaches the reader under "attach://<filename>" so the file bytes are
// sent with the request; URL and file-id strings are passed through as-is.
func inputMediaPhoto(m *Message) *models.InputMediaPhoto {
	photo := &models.InputMediaPhoto{
		Caption:   m.Text,
		ParseMode: m.ParseMode,
	}
	switch media := m.Media.(type) {
	case *models.InputFileUpload:
		photo.Media = "attach://" + media.Filename
		photo.MediaAttachment = media.Data
	case *models.InputFileString:
		photo.Media = media.Data
	}
	return photo
}

// toEditMessageMediaParams builds the media edit params regardless of whether the
// target message currently has a photo. Telegram upgrades a plain text message
// when editing it with an InputMedia. ChatID must be non-nil and messageID > 0.
func toEditMessageMediaParams(chatID any, messageID int, m *Message) *bot.EditMessageMediaParams {
	params := &bot.EditMessageMediaParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: m.replyMarkup(),
	}
	if m.Media != nil {
		params.Media = inputMediaPhoto(m)
	}
	return params
}

// hasMedia reports whether the message carries media that can only be edited
// through its caption (a caption edit on a plain text message is rejected by
// Telegram). Chat events such as pinned or member-service messages are not
// counted as media.
func hasMedia(m *models.Message) bool {
	return len(m.Photo) > 0 ||
		m.Video != nil ||
		m.VideoNote != nil ||
		m.Document != nil ||
		m.Animation != nil ||
		m.Audio != nil ||
		m.Voice != nil ||
		m.Sticker != nil
}

func (m *Message) toSendMessageParams(chatID int64) *bot.SendMessageParams {
	params := &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        m.Text,
		ParseMode:   m.ParseMode,
		ReplyMarkup: m.replyMarkup(),
	}
	return params
}

func (m *Message) toSendPhotoParams(chatID int64) *bot.SendPhotoParams {
	params := &bot.SendPhotoParams{
		ChatID:      chatID,
		Photo:       m.Media,
		Caption:     m.Text,
		ParseMode:   m.ParseMode,
		ReplyMarkup: m.replyMarkup(),
	}
	return params
}

func (m *Message) toEditMessageTextParams(chatID int64, messageID int) *bot.EditMessageTextParams {
	params := &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        m.Text,
		ParseMode:   m.ParseMode,
		ReplyMarkup: m.replyMarkup(),
	}
	return params
}

func (m *Message) toEditMessageCaptionParams(chatID int64, messageID int) *bot.EditMessageCaptionParams {
	params := &bot.EditMessageCaptionParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Caption:     m.Text,
		ParseMode:   m.ParseMode,
		ReplyMarkup: m.replyMarkup(),
	}
	return params
}
