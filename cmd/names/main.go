package main

import (
	"log"
	"os"

	dataset "github.com/curtiscovington/ssa-names/data/namesbystate"
	"github.com/curtiscovington/ssa-names/internal/cli"
)

func main() {
	app := cli.NewApp(dataset.Files, os.Stdout, os.Stderr)
	if err := app.Run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
