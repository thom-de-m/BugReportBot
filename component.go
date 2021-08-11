package main

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

const (
	bugReportButtonID = "report_btn"
)

var (
	interactionResponseCommand *discordgo.InteractionResponse
	interactionNotAllowed      *discordgo.InteractionResponse
)

func initResponses() {
	interactionResponseCommand = &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
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
		},
	}

	interactionNotAllowed = &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: config.Messages.InteractionNotAllowed,
		},
	}
}

// This takes care of the slash command and interactions for the button that can be setup
func handleInteractions(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	switch interaction.Type {
	case discordgo.InteractionApplicationCommand:
		// This command can only be used in the Guild
		if interaction.Member == nil {
			session.InteractionRespond(interaction.Interaction, interactionNotAllowed)
			return
		}

		permissions, permissionErr := session.UserChannelPermissions(interaction.Member.User.ID, interaction.ChannelID)
		if permissionErr != nil {
			log.Println("Unable to check permissions for channel..")
			log.Println(permissionErr)
			session.InteractionRespond(interaction.Interaction, interactionNotAllowed)
			return
		}

		if (permissions & discordgo.PermissionAdministrator) != 0 {
			// The user is not an administrator and is not allowed to use this!
			session.InteractionRespond(interaction.Interaction, interactionNotAllowed)
			return
		}

		if err := session.InteractionRespond(interaction.Interaction, interactionResponseCommand); err != nil {
			log.Println("Unable to create slash command response!")
			log.Println(err)
		}
	case discordgo.InteractionMessageComponent:
		if interaction.MessageComponentData().CustomID != bugReportButtonID {
			return
		}

		if interaction.Member == nil {
			session.InteractionRespond(interaction.Interaction, interactionNotAllowed)
			return
		}

		// Handle the bug button click!
		startNewReportConversation(interaction.Member.User.ID, interaction.ChannelID)

		// Reply to Discord with that everything succeeded
		session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
	}
}
