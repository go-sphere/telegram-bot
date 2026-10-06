// Package tmaauth validates and creates Telegram Mini App init data, the
// signed query string a Mini App receives in window.Telegram.WebApp.initData.
//
// Create a validator with [NewTmaAuth] using the bot token that issued the
// init data and a maximum age, then call [TmaAuth.ParseToken] on the raw init
// data sent by the client. Clients commonly send it in an Authorization header
// as "tma <initData>" (see [AuthorizationPrefixTMA]); strip the scheme before
// parsing. A TmaAuth holds no resources and needs no cleanup.
//
// # Usage
//
//	import (
//		"errors"
//		"net/http"
//		"os"
//		"strings"
//		"time"
//
//		"github.com/go-sphere/telegram-bot/tmaauth"
//	)
//
//	var auth = tmaauth.NewTmaAuth(os.Getenv("TELEGRAM_BOT_TOKEN"), 24*time.Hour)
//
//	func userID(r *http.Request) (int64, error) {
//		initData, ok := strings.CutPrefix(r.Header.Get("Authorization"), tmaauth.AuthorizationPrefixTMA+" ")
//		if !ok {
//			return 0, errors.New("missing tma authorization")
//		}
//		claims, err := auth.ParseToken(r.Context(), initData)
//		if err != nil {
//			return 0, err // invalid signature, expired, or malformed
//		}
//		return claims.User.ID, nil
//	}
//
// [TmaAuth.GenerateToken] signs [Claims] with the same token; it is mainly
// useful for tests and local development, because real init data is created
// by Telegram.
//
// Validation errors come from github.com/telegram-mini-apps/init-data-golang
// and can be matched with errors.Is against its exported errors, such as
// ErrSignInvalid and ErrExpired.
package tmaauth
