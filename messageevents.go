package main

import (
	"github.com/bwmarrin/discordgo"
)

// handles all incoming messages for the bot. Note that this also includes
// certain criteria we don't want. Such as other bot messages, or even the guild messages.
// All we care about is Direct Messages!
func handleIncomingMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	// Filter out non Direct Message messages
	channel, channelErr := session.Channel(message.ChannelID)
	if channelErr != nil {
		return
	}

	if channel.Type != discordgo.ChannelTypeDM {
		return
	}

	// Filters out messages from other bots, but also our own!
	if message.Author.Bot {
		return
	}

	// Check if this person already is in an ongoing conversation with the bot
	currentReportsMutex.RLock()
	if _, ok := currentOngoingReports[message.Author.ID]; ok {
		// The user is already in an ongoing conversation, continue it
		currentReportsMutex.RUnlock()
	} else {
		// The user is not in an ongoing conversation, make sure to start a new one
		currentReportsMutex.RUnlock()
		startNewReportConversation(message.Author.ID, "")
	}
}
