package session

import "sync"

const CurrentUserID = 2

type User struct{ ID int }

var current = sync.OnceValue(func() User { return User{ID: CurrentUserID} })

func CurrentUser() User { return current() }
