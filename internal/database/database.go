package database

import (
	"errors"
	"net"
	"net/url"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect() (*gorm.DB, error) {
	password := os.Getenv("PGPASSWORD")
	if password == "" {
		return nil, errors.New("задайте пароль PostgreSQL в переменной окружения PGPASSWORD")
	}

	address := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(setting("PGUSER", "postgres"), password),
		Host:   net.JoinHostPort(setting("PGHOST", "localhost"), setting("PGPORT", "5432")),
		Path:   "/" + setting("PGDATABASE", "rip_lab2"),
	}
	options := url.Values{}
	options.Set("sslmode", setting("PGSSLMODE", "disable"))
	options.Set("connect_timeout", "5")
	address.RawQuery = options.Encode() + "&TimeZone=Europe/Moscow"

	return gorm.Open(postgres.Open(address.String()), &gorm.Config{})
}

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
