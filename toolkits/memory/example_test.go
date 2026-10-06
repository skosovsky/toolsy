package memory_test

import (
	"fmt"

	"github.com/skosovsky/toolsy/toolkits/memory"
)

func ExampleNewScratchpad() {
	pad, err := memory.NewScratchpad(memory.WithMaxFacts(100))
	if err != nil {
		panic(err)
	}
	tools, err := pad.AsTools()
	if err != nil {
		panic(err)
	}
	fmt.Println(len(tools))
	// Output: 3
}
