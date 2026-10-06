package human_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/human"
)

func ExampleAsTools() {
	tools, err := human.AsTools()
	if err != nil {
		panic(err)
	}
	err = tools[0].Execute(
		context.Background(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"delete document","reason":"request review"}`)},
		func(chunk toolsy.Chunk) error {
			var intent map[string]string
			decodeErr := json.Unmarshal([]byte(chunk.Control.(*toolsy.PauseSignal).Reason), &intent)
			if decodeErr != nil {
				return decodeErr
			}
			fmt.Println(tools[0].Manifest().Name, intent["kind"])
			return nil
		},
	)
	fmt.Println("pause:", errors.Is(err, toolsy.ErrPause), "grant issued: false")
	// Output:
	// request_human_review human_review
	// pause: true grant issued: false
}
