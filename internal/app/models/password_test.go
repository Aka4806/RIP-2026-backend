package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserPassword(t *testing.T) {
	var user User
	if err := user.SetPassword("demo-password"); err != nil {
		t.Fatal(err)
	}
	if user.Password == "demo-password" || !user.CheckPassword("demo-password") || user.CheckPassword("wrong-password") {
		t.Fatal("password must be hashed and checked correctly")
	}
	previous := user.Password
	if err := user.SetPassword(""); err == nil {
		t.Fatal("empty password must be rejected")
	}
	if err := user.SetPassword(strings.Repeat("x", 73)); err == nil {
		t.Fatal("password exceeding bcrypt byte limit must be rejected")
	}
	if user.Password != previous {
		t.Fatal("failed password change must preserve the existing hash")
	}
	data, err := json.Marshal(user)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), user.Password) {
		t.Fatal("JSON must not expose the password hash")
	}
}
