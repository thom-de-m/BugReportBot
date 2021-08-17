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
	userCooldownsForReportButton = make(map[string]time.Time)
	userCooldownsMutex           = new(sync.RWMutex)
)

func addUserToCooldownForReportButton(userID string) {
	userCooldownsMutex.Lock()
	defer userCooldownsMutex.Unlock()

	userCooldownsForReportButton[userID] = time.Now().Add(time.Duration(config.ReportButtonCooldownSeconds) * time.Second)
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

		// Check if the user isn't on a cooldown!
		userCooldownsMutex.RLock()
		defer userCooldownsMutex.RUnlock()

		if cooldown, ok := userCooldownsForReportButton[interaction.Member.User.ID]; ok {
			// If this validates true then the user is still on a cooldown
			if time.Now().Before(cooldown) {
				return
			}
		}

		// Handle the bug button click!
		go startNewReportConversation(interaction.Member.User.ID, interaction.ChannelID)
	}
}
