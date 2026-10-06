package toolsy_test

import (
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

func ExamplePut() {
	// Dependencies can be installed without binding a state session.
	env := toolsy.NewRunEnv(nil)
	if err := toolsy.Put(env, "host", "tenant-a"); err != nil {
		panic(err)
	}
	host, err := toolsy.Require[string](env, "host")
	if err != nil {
		panic(err)
	}
	fmt.Println(host)
	// State mutation needs an explicitly bound Session.
	err = toolsy.SetState(env, "counter", 1)
	fmt.Println(errors.Is(err, toolsy.ErrMutationConfiguration))
	// Output:
	// tenant-a
	// true
}
