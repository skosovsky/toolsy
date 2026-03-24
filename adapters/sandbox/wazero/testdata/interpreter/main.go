package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	codeBytes, err := readWorkspaceFile("main.code")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	code := strings.TrimSpace(string(codeBytes))
	switch {
	case strings.HasPrefix(code, "stdout:"):
		fmt.Fprint(os.Stdout, strings.TrimPrefix(code, "stdout:"))
	case strings.HasPrefix(code, "env:"):
		fmt.Fprint(os.Stdout, os.Getenv(strings.TrimPrefix(code, "env:")))
	case strings.HasPrefix(code, "file:"):
		name := strings.TrimPrefix(code, "file:")
		data, err := readWorkspaceFile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		fmt.Fprint(os.Stdout, string(data))
	case strings.HasPrefix(code, "exit:"):
		fmt.Fprintln(os.Stderr, "requested exit")
		value := strings.TrimPrefix(code, "exit:")
		exitCode, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
		os.Exit(exitCode)
	case code == "sleep":
		time.Sleep(5 * time.Second)
		fmt.Fprint(os.Stdout, "done")
	default:
		fmt.Fprint(os.Stdout, code)
	}
}

func readWorkspaceFile(name string) ([]byte, error) {
	candidates := []string{
		"/workspace/" + name,
		"workspace/" + name,
		name,
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
	}
	return nil, errors.New("workspace file not found: " + name)
}
