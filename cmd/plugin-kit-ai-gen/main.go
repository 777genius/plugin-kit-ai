package main

import (
	"log"
	"os"

	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/generator"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	root, err := generator.FindRepoRoot(cwd)
	if err != nil {
		log.Fatal(err)
	}
	if err := generator.WriteAll(root); err != nil {
		log.Fatal(err)
	}
}
