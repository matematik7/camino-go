package models

import (
	"github.com/jinzhu/gorm"
	"github.com/matematik7/gongo/authorization"
)

type DiaryEntry struct {
	gorm.Model
	Title      string             `valid:"required"`
	Text       string             `gorm:"type:text" valid:"required"`
	Author     authorization.User `valid:"-"`
	AuthorID   uint               `valid:"required"`
	Comments   []Comment          `valid:"-"`
	MapEntry   MapEntry           `valid:"-"`
	MapEntryID uint

	NumComments uint `gorm:"-"`
	Viewed      bool `gorm:"-"`
	NewComments bool `gorm:"-"`
}

type Comment struct {
	gorm.Model
	DiaryEntryID uint
	Comment      string             `gorm:"type:text" valid:"required"`
	Author       authorization.User `valid:"-"`
	AuthorID     uint
}

type EntryUserRead struct {
	gorm.Model
	DiaryEntryID uint
	UserID       uint
}