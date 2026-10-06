package tmaauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	initdata "github.com/telegram-mini-apps/init-data-golang"
)

// AuthorizationPrefixTMA is the Authorization header scheme used by Telegram
// Mini App clients: "Authorization: tma <initData>". The package does not
// parse headers; callers strip the scheme and the following space before
// calling TmaAuth.ParseToken.
const AuthorizationPrefixTMA = "tma"

// Claims is the parsed Mini App init data, an alias for
// github.com/telegram-mini-apps/init-data-golang.InitData. User.ID identifies
// the Telegram user.
type Claims = initdata.InitData

// TmaAuth validates and signs Mini App init data for one bot token.
// Create it with NewTmaAuth; the zero value has an empty token and validates
// nothing useful. A TmaAuth is immutable and safe for concurrent use.
type TmaAuth struct {
	token string
	expIn time.Duration
}

// NewTmaAuth returns a TmaAuth for the bot token that issued the init data.
// expIn is the maximum accepted age of init data, measured from its auth_date;
// zero or negative disables the expiration check, which is not recommended
// for production.
func NewTmaAuth(token string, expIn time.Duration) *TmaAuth {
	return &TmaAuth{
		token: token,
		expIn: expIn,
	}
}

// ParseToken validates the raw init data query string token (without the
// "tma " scheme) and returns its parsed claims.
//
// It returns an error, with zero Claims, when the query is malformed, the hash
// is missing or does not match the bot token, auth_date is missing or invalid
// while an expiration is configured, or the data is older than expIn. These
// errors wrap or equal the exported errors of
// github.com/telegram-mini-apps/init-data-golang (for example ErrSignInvalid,
// ErrExpired, ErrSignMissing); match them with errors.Is. ctx is not used.
func (t *TmaAuth) ParseToken(ctx context.Context, token string) (Claims, error) {
	err := initdata.Validate(token, t.token, t.expIn)
	if err != nil {
		return Claims{}, err
	}
	data, err := initdata.Parse(token)
	if err != nil {
		return Claims{}, err
	}
	return data, nil
}

// GenerateToken signs claims with the bot token and returns init data in the
// query string format accepted by ParseToken. claims.AuthDateRaw (Unix
// seconds) becomes auth_date; claims.Hash is ignored. Real init data is
// created by Telegram, so this is mainly useful for tests and local tools.
//
// It returns an error when claims is nil or cannot be encoded. ctx is not used.
func (t *TmaAuth) GenerateToken(ctx context.Context, claims *Claims) (string, error) {
	if claims == nil {
		return "", fmt.Errorf("claims must not be nil")
	}
	rawInitMap := map[string]any{}
	initBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(initBytes))
	decoder.UseNumber()
	err = decoder.Decode(&rawInitMap)
	if err != nil {
		return "", err
	}
	delete(rawInitMap, "hash")
	delete(rawInitMap, "auth_date")
	params := make(map[string]string)
	values := url.Values{}
	for k, v := range rawInitMap {
		if str, ok := v.(string); ok {
			params[k] = v.(string)
			values.Add(k, str)
			continue
		}
		if num, ok := v.(json.Number); ok {
			params[k] = num.String()
			values.Add(k, num.String())
			continue
		}
		partBytes, e := json.Marshal(v)
		if e != nil {
			return "", e
		}
		params[k] = string(partBytes)
		values.Add(k, string(partBytes))
	}
	exp := time.Unix(int64(claims.AuthDateRaw), 0)
	sign := initdata.Sign(params, t.token, exp)
	values.Set("hash", sign)
	values.Set("auth_date", strconv.FormatInt(exp.Unix(), 10))
	return values.Encode(), nil
}
