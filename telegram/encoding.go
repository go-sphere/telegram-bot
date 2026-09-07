package telegram

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-sphere/jsoncompressor"
)

// MaxCallbackDataLength is the maximum payload size, in bytes, that Telegram
// accepts for inline keyboard callback data.
const MaxCallbackDataLength = 64

// ErrCallbackDataTooLong is returned when the marshaled callback data exceeds
// MaxCallbackDataLength.
var (
	ErrCallbackDataTooLong = errors.New("telegram: callback data exceeds 64 bytes")
	errRouteEmpty          = errors.New("telegram: route must not be empty")
	errRouteContainsColon  = errors.New("telegram: route must not contain ':'")
)

// callback data format: $route:compressed_json($data)
// the route prefix must not contain ":", which is used to separate the parts.

// UnmarshalData decodes Telegram callback query data into a route and typed data structure.
// The input data should be formatted as "route:compressed_json_data".
// Returns the route string, the unmarshaled data of type T, and any error encountered.
// This is commonly used for handling Telegram bot callback queries with structured data.
func UnmarshalData[T any](data string) (string, *T, error) {
	cmp := strings.SplitN(data, ":", 2)
	if len(cmp) != 2 {
		return "", nil, fmt.Errorf("invalid data format")
	}
	var v T
	err := jsoncompressor.Unmarshal([]byte(cmp[1]), &v)
	if err != nil {
		return cmp[0], nil, err
	}
	return cmp[0], &v, nil
}

// MarshalData encodes a route and typed data into Telegram callback query format.
// The data is compressed using JSON compression and formatted as "route:compressed_json_data".
// The route must be non-empty and must not contain ":", and the resulting string
// must fit into the 64 bytes Telegram allows for callback data.
func MarshalData[T any](route string, data T) (string, error) {
	if route == "" {
		return "", errRouteEmpty
	}
	if strings.Contains(route, ":") {
		return "", fmt.Errorf("%w: got %q", errRouteContainsColon, route)
	}
	b, err := jsoncompressor.Marshal(data)
	if err != nil {
		return "", err
	}
	callback := route + ":" + string(b)
	if len(callback) > MaxCallbackDataLength {
		return "", fmt.Errorf("%w: got %d bytes", ErrCallbackDataTooLong, len(callback))
	}
	return callback, nil
}
