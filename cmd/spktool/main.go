package main

import (
	"context"
	"os"

	"github.com/mnutt/spktool/internal/app"
)

func main() {
	os.Exit(app.Run(context.Background(), os.Args))
}
