package main

import (
	"fmt"
	"log"

	"awesomeProject/internal/api"
	"awesomeProject/internal/database"
)

// @title SQL Index API ЛР4
// @version 4.0
// @description JWT авторизация и Redis blacklist. В Authorize введите Bearer и токен через пробел.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
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
