package main

import (
	"fmt"
	"log"

	"github.com/peterjohnbishop/centra-chatter/server"
	"github.com/peterjohnbishop/centra-chatter/storage"
)

func main() {
	fmt.Println("central-chatter")

	db, err := storage.NewStorage("./db/local.db")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	server.ServeWish(db)
}
