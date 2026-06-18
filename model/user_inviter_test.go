package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAttachInviterUsernames(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("failed to migrate users: %v", err)
	}

	inviter := User{Username: "alice", Password: "password123"}
	if err := db.Create(&inviter).Error; err != nil {
		t.Fatalf("failed to create inviter: %v", err)
	}

	users := []*User{
		{Id: 2, Username: "bob", InviterId: inviter.Id},
		{Id: 3, Username: "carol", InviterId: inviter.Id},
		{Id: 4, Username: "dave", InviterId: 0},
	}

	if err := attachInviterUsernames(db, users); err != nil {
		t.Fatalf("attachInviterUsernames returned error: %v", err)
	}

	if users[0].InviterUsername != inviter.Username {
		t.Fatalf("users[0].InviterUsername = %q, want %q", users[0].InviterUsername, inviter.Username)
	}
	if users[1].InviterUsername != inviter.Username {
		t.Fatalf("users[1].InviterUsername = %q, want %q", users[1].InviterUsername, inviter.Username)
	}
	if users[2].InviterUsername != "" {
		t.Fatalf("users[2].InviterUsername = %q, want empty", users[2].InviterUsername)
	}
}
