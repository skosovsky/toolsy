package main

import (
	"context"
	"fmt"

	"github.com/skosovsky/toolsy"
)

//go:generate go run ../../cmd/toolsy-gen presence.json

type handler struct{}

func (handler) Execute(_ context.Context, input PresenceInput) (string, error) {
	return fmt.Sprintf("text=%q integer=%s active=%t tags=%d optionalTextOmitted=%t nullable=%s",
		input.RString, input.RInteger.String(), *input.RBoolean, len(*input.RStrings),
		input.OString == nil, input.RNullableString), nil
}

func main() {
	tool, err := NewPresenceTool(handler{})
	if err != nil {
		panic(err)
	}
	err = tool.Execute(context.Background(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(exampleInput)},
		func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil })
	if err != nil {
		panic(err)
	}
}

const exampleInput = `{"r_string":"","r_integer":9007199254740993,"r_boolean":false,
 "r_datetime":"2026-10-07T00:00:00Z","r_datetimes":[],"r_strings":[],"r_integers":[],"r_booleans":[],
 "r_nullable_string":null,"r_nullable_integer":null,"r_nullable_boolean":null,
 "r_nullable_strings":null,"r_nullable_integers":null,"r_nullable_booleans":null}`
