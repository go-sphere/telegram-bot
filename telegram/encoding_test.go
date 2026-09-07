package telegram

import (
	"errors"
	"strings"
	"testing"
)

type testDataStruct struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

func TestMarshalData(t *testing.T) {
	data, err := MarshalData[testDataStruct]("test", testDataStruct{
		Number: 123,
		Text:   "456",
	})
	if err != nil {
		t.Fatalf("MarshalData failed: %v", err)
	}
	target := `test:[123,"456"]`
	if data != target {
		t.Errorf("Marshaled data is invalid, got: %s", data)
	}
}

func TestMarshalDataErrors(t *testing.T) {
	tests := []struct {
		name  string
		route string
		data  any
		want  error
	}{
		{
			name:  "empty route",
			route: "",
			data:  testDataStruct{},
			want:  errRouteEmpty,
		},
		{
			name:  "route contains colon",
			route: "a:b",
			data:  testDataStruct{},
			want:  errRouteContainsColon,
		},
		{
			name:  "payload too long",
			route: "test",
			data:  testDataStruct{Text: strings.Repeat("x", 200)},
			want:  ErrCallbackDataTooLong,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MarshalData[any](tt.route, tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("MarshalData error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUnmarshalData(t *testing.T) {
	raw := `test:[123,"456"]`
	route, data, err := UnmarshalData[testDataStruct](raw)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if data == nil {
		t.Fatal("Unmarshaled data is nil")
	}
	if route != "test" {
		t.Errorf("Unmarshaled route is invalid, got: %s", route)
	}
	if data.Number != 123 {
		t.Errorf("Unmarshaled number is invalid, got: %d", data.Number)
	}
	if data.Text != "456" {
		t.Errorf("Unmarshaled text is invalid, got: %s", data.Text)
	}
}

func TestUnmarshalDataInvalidFormat(t *testing.T) {
	if _, _, err := UnmarshalData[testDataStruct]("no-separator"); err == nil {
		t.Fatal("expected an error for data without a separator")
	}
}
