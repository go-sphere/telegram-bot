package tmaauth

import (
	"context"
	"errors"
	"testing"
	"time"

	initdata "github.com/telegram-mini-apps/init-data-golang"
)

func TestTmaAuth_ParseToken(t *testing.T) {
	ctx := context.Background()
	secretToken := "test"
	claims := Claims{
		ChatInstance:    123,
		CanSendAfterRaw: 1234,
		User: initdata.User{
			AddedToAttachmentMenu: false,
			AllowsWriteToPm:       false,
			FirstName:             "full_name",
			ID:                    1,
			IsBot:                 false,
			IsPremium:             false,
			LastName:              "last_name",
			Username:              "username",
			LanguageCode:          "",
			PhotoURL:              "",
		},
		AuthDateRaw: int(time.Now().Unix()),
	}
	tmaAuth := NewTmaAuth(secretToken, time.Hour)
	token, err := tmaAuth.GenerateToken(ctx, &claims)
	if err != nil {
		t.Error(err)
	}
	t.Log(token)
	parsedClaims, err := tmaAuth.ParseToken(ctx, token)
	if err != nil {
		t.Error(err)
		return
	}
	if parsedClaims.User.ID != claims.User.ID {
		t.Errorf("expected %d, got %d", claims.User.ID, parsedClaims.User.ID)
	}
}

func TestTmaAuth_EmptyTokenRejected(t *testing.T) {
	ctx := context.Background()
	for _, auth := range []*TmaAuth{NewTmaAuth("", time.Hour), {}} {
		if _, err := auth.ParseToken(ctx, "user=%7B%22id%22%3A1%7D&hash=x"); !errors.Is(err, ErrEmptyBotToken) {
			t.Errorf("ParseToken error = %v, want ErrEmptyBotToken", err)
		}
		if _, err := auth.GenerateToken(ctx, &Claims{}); !errors.Is(err, ErrEmptyBotToken) {
			t.Errorf("GenerateToken error = %v, want ErrEmptyBotToken", err)
		}
	}
}
