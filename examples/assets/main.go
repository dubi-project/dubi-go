package main

import (
	"log"

	dubi "github.com/dubi-project/dubi-go"
	_ "github.com/dubi-project/dubi-go/web"
)

func main() {
	if err := dubi.Run(); err != nil {
		log.Fatal(err)
	}
}
