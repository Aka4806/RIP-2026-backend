package main

import (
	"log"

	"awesomeProject/internal/api"
)

func main() {
	if err := api.StartServer(); err != nil {
		log.Fatal(err)
	}
}
