// Command decodegen writes the discord package's decode_gen.go: go generate ./discord
package main

import (
	"fmt"
	"os"

	"github.com/disgoorg/disgo/internal/decodegen"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := decodegen.Write(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
