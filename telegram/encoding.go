package telegram

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-sphere/jsoncompressor"
)

// MaxCallbackDataLength is the maximum payload size, in bytes, that Telegram
// accepts for inline keyboard callback data. MarshalData and NewButton reject
// longer data.
const MaxCallbackDataLength = 64

// ErrCallbackDataTooLong is returned, wrapped, when the marshaled callback
// data exceeds MaxCallbackDataLength; test for it with errors.Is.
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
//
// The payload is decoded with github.com/go-sphere/jsoncompressor, so T must
// have the same tagged fields, in the same order, as the value given to
// MarshalData. Data without ":" returns an error and an empty route. When the payload
// cannot be decoded into T, the route is still returned with a nil value and
// the decoding error.
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
// A too long result returns an error wrapping ErrCallbackDataTooLong; JSON
// encoding errors for data are returned as is.
//
// Data is encoded with github.com/go-sphere/jsoncompressor: struct values
// become JSON arrays in field order and only exported fields with a json tag
// are included, so untagged fields are silently dropped. Reordering the fields
// of T breaks callback data created before the change.
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
