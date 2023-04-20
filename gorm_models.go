package main

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	gorm.Model
	Username string `gorm:"primaryKey"`
	Password string

	Subscription time.Time // valid until
	IsAdmin      bool
}

type Transaction struct {
	gorm.Model
	PayId    string
	Uid      string
	IsActive bool
	PayUrl   string
}
