package main

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	bugReportButtonID = "report_btn"
)

var (
	userCooldownsMessages      = make(map[string]time.Time)
	userCooldownsMessagesMutex = new(sync.RWMutex)
)

func setAndCheckCooldownForUserMessages(userID string) (onCooldown bool) {
	userCooldownsMessagesMutex.Lock()
	defer userCooldownsMessagesMutex.Unlock()

	onCooldown = isUserOnCooldownForMessages(userID, false)
	if onCooldown {
		return onCooldown
	}

	setCooldownForUserMessages(userID, false)
	return
}

func setCooldownForUserMessages(userID string, lock bool) {
	if lock {
		userCooldownsMessagesMutex.Lock()
		defer userCooldownsMessagesMutex.Unlock()
	}

	userCooldownsMessages[userID] = time.Now().Add(time.Duration(config.ReportMessagesCooldownSeconds) * time.Second)
}

func isUserOnCooldownForMessages(userID string, lock bool) bool {
	if lock {
		userCooldownsMessagesMutex.RLock()
		defer userCooldownsMessagesMutex.RUnlock()
	}

	if cooldown, ok := userCooldownsMessages[userID]; ok {
		// If this validates true then the user is still on a cooldown
		if time.Now().Before(cooldown) {
			return true
		}
	}

	return false
}

// This takes care of the slash command and interactions for the button that can be setup
func handleInteractions(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	switch interaction.Type {
	case discordgo.InteractionMessageComponent:
		if interaction.MessageComponentData().CustomID != bugReportButtonID {
			return
		}

		// From here we should always respond to Discord that we at least received the event and handled it accordingly
		defer session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})

		if interaction.Member == nil {
			return
		}

		// Handle the bug button click!
		go startNewReportConversation(interaction.Member.User.ID, interaction.ChannelID)
	}
}
