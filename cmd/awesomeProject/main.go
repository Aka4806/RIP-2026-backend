package main

import (
	"fmt"
	"log"

	"awesomeProject/internal/api"
	"awesomeProject/internal/database"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	db, err := database.Connect()
	if err != nil {
		return fmt.Errorf("ошибка подключения к PostgreSQL: %w", err)
	}
	connection, err := db.DB()
	if err != nil {
		return fmt.Errorf("ошибка получения соединения PostgreSQL: %w", err)
	}
	defer connection.Close()

	log.Println("Подключение к PostgreSQL успешно")

	return api.StartServer(db)
}
