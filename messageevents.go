package main

import (
	"github.com/bwmarrin/discordgo"
)

// handles all incoming messages for the bot. Note that this also includes
// certain criteria we don't want. Such as other bot messages, or even the guild messages.
// All we care about is Direct Messages!
func handleIncomingMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	// Filters out messages from other bots, but also our own!
	if message.Author.Bot {
		return
	}

	// Sneaky way to handle message components
	sendInteractionComponentIfNeeded(message)

	// Filter out non Direct Message messages
	channel, channelErr := session.Channel(message.ChannelID)
	if channelErr != nil {
		return
	}

	if channel.Type != discordgo.ChannelTypeDM {
		return
	}

	// Check if this person already is in an ongoing conversation with the bot
	currentReportsMutex.RLock()
	if report, ok := currentOngoingReports[message.Author.ID]; ok {
		currentReportsMutex.RUnlock()
		report.lock.Lock()
		defer report.lock.Unlock()

		// The user is already in an ongoing conversation, continue it
		continueOngoingReport(report, message.Content, message.Author.ID)
	} else {
		// The user is not in an ongoing conversation, make sure to start a new one
		currentReportsMutex.RUnlock()
		startNewReportConversation(message.Author.ID, "")
	}
}

func sendInteractionComponentIfNeeded(message *discordgo.MessageCreate) {
	if message.ChannelID != config.SubmitReportChannelID {
		return
	}

	botSession.ChannelMessageSendComplex(message.ChannelID, &discordgo.MessageSend{
		Content: config.Messages.InteractionButtonContent,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						CustomID: bugReportButtonID,
						Label:    "Start A Report",
						Style:    discordgo.PrimaryButton,
					},
				},
			},
		},
	})
}
