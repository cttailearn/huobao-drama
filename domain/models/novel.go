package models

import (
	"time"

	"gorm.io/gorm"
)

type Novel struct {
	ID              uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	DramaID         uint           `gorm:"not null;default:0;index" json:"drama_id"`
	Title           string         `gorm:"type:varchar(200);not null" json:"title"`
	Genre           string         `gorm:"type:varchar(100);not null" json:"genre"`
	ChapterCount    int            `gorm:"not null;default:1" json:"chapter_count"`
	WordsPerChapter int            `gorm:"not null;default:1500" json:"words_per_chapter"`
	Requirement     string         `gorm:"type:text" json:"requirement"`
	Status          string         `gorm:"type:varchar(30);not null;default:'draft'" json:"status"`
	SetupContent    *string        `gorm:"type:longtext" json:"setup_content"`
	OutlineContent  *string        `gorm:"type:longtext" json:"outline_content"`
	CurrentChapter  int            `gorm:"not null;default:0" json:"current_chapter"`
	CreatedAt       time.Time      `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"not null;autoUpdateTime" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	Chapters []NovelChapter `gorm:"foreignKey:NovelID" json:"chapters,omitempty"`
}

func (n *Novel) TableName() string {
	return "novels"
}

type NovelChapter struct {
	ID            uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	NovelID       uint           `gorm:"not null;index" json:"novel_id"`
	ChapterNumber int            `gorm:"not null;index" json:"chapter_number"`
	Title         string         `gorm:"type:varchar(200);not null" json:"title"`
	Outline       *string        `gorm:"type:text" json:"outline"`
	DraftContent  *string        `gorm:"type:longtext" json:"draft_content"`
	FinalContent  *string        `gorm:"type:longtext" json:"final_content"`
	Status        string         `gorm:"type:varchar(30);not null;default:'pending'" json:"status"`
	CreatedAt     time.Time      `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null;autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (n *NovelChapter) TableName() string {
	return "novel_chapters"
}
