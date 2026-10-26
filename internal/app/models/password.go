package models

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

func (user *User) SetPassword(password string) error {
	if password == "" {
		return errors.New("пароль не должен быть пустым")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return nil
}

func (user User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil
}
