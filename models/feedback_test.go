package models

import (
	"reflect"
	"testing"
)

func TestIntOrNil(t *testing.T) {
	positive := 7
	zero := 0
	negative := -3
	positive64 := int64(9)
	zero64 := int64(0)
	positive64Ptr := int64(5)

	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{name: "nil value", input: nil, want: nil},
		{name: "positive int", input: 5, want: 5},
		{name: "zero int", input: zero, want: nil},
		{name: "negative int", input: negative, want: nil},
		{name: "positive int pointer", input: &positive, want: positive},
		{name: "nil int pointer", input: (*int)(nil), want: nil},
		{name: "non-positive int pointer", input: &zero, want: nil},
		{name: "positive int64", input: positive64, want: positive64},
		{name: "zero int64", input: zero64, want: nil},
		{name: "positive int64 pointer", input: &positive64Ptr, want: positive64Ptr},
		{name: "nil int64 pointer", input: (*int64)(nil), want: nil},
		{name: "float64 value", input: 4.0, want: int64(4)},
		{name: "non-positive float64", input: -2.0, want: nil},
	}

	for _, tc := range tests {
		got := intOrNil(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: expected %v (%T), got %v (%T)", tc.name, tc.want, tc.want, got, got)
		}
	}
}

func TestStrOrNil(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{name: "nil", input: nil, want: nil},
		{name: "empty string", input: "", want: nil},
		{name: "whitespace string", input: "  \t", want: nil},
		{name: "non-empty string", input: "  hello  ", want: "hello"},
		{name: "non-string value", input: 123, want: 123},
	}

	for _, tc := range tests {
		got := strOrNil(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: expected %v (%T), got %v (%T)", tc.name, tc.want, tc.want, got, got)
		}
	}
}
