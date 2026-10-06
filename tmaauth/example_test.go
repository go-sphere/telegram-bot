package tmaauth_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-sphere/telegram-bot/tmaauth"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

const botToken = "123456:test-bot-token"

// signedInitData builds init data the way Telegram does, signing it with
// botToken, so the examples run without a real Mini App client.
func signedInitData(user string, authDate time.Time) string {
	hash := initdata.Sign(map[string]string{"user": user}, botToken, authDate)
	q := url.Values{}
	q.Set("user", user)
	q.Set("auth_date", strconv.FormatInt(authDate.Unix(), 10))
	q.Set("hash", hash)
	return q.Encode()
}

func ExampleTmaAuth_ParseToken() {
	auth := tmaauth.NewTmaAuth(botToken, time.Hour)
	initData := signedInitData(`{"id":42,"first_name":"Ada","username":"ada"}`, time.Now())

	claims, err := auth.ParseToken(context.Background(), initData)
	if err != nil {
		fmt.Println("invalid init data:", err)
		return
	}
	fmt.Println(claims.User.ID, claims.User.Username)
	// Output: 42 ada
}

func ExampleTmaAuth_ParseToken_invalid() {
	auth := tmaauth.NewTmaAuth(botToken, time.Hour)

	// Signed by a different bot token.
	other := tmaauth.NewTmaAuth("654321:other-token", time.Hour)
	foreign, err := other.GenerateToken(context.Background(), &tmaauth.Claims{
		User:        initdata.User{ID: 42},
		AuthDateRaw: int(time.Now().Unix()),
	})
	if err != nil {
		fmt.Println("generate:", err)
		return
	}
	_, err = auth.ParseToken(context.Background(), foreign)
	fmt.Println("bad signature:", errors.Is(err, initdata.ErrSignInvalid))

	// Correctly signed, but older than the one hour limit.
	stale := signedInitData(`{"id":42}`, time.Now().Add(-2*time.Hour))
	_, err = auth.ParseToken(context.Background(), stale)
	fmt.Println("expired:", errors.Is(err, initdata.ErrExpired))
	// Output:
	// bad signature: true
	// expired: true
}

func ExampleTmaAuth_GenerateToken() {
	auth := tmaauth.NewTmaAuth(botToken, time.Hour)
	token, err := auth.GenerateToken(context.Background(), &tmaauth.Claims{
		User:        initdata.User{ID: 7, Username: "tester"},
		AuthDateRaw: int(time.Now().Unix()),
	})
	if err != nil {
		fmt.Println("generate:", err)
		return
	}
	// A test client would send: "Authorization: tma " + token
	claims, err := auth.ParseToken(context.Background(), token)
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	fmt.Println(claims.User.ID, claims.User.Username)
	// Output: 7 tester
}
