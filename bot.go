package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	config *basicConfig

	botSession *discordgo.Session
)

var (
	currentOngoingReports = make(map[string]*reportData)
	currentReportsMutex   = new(sync.RWMutex)
)

func init() {
	fileBytes, fileErr := ioutil.ReadFile(filepath.FromSlash("./config/config.json"))
	if fileErr != nil {
		log.Println("Unable to find file \"config.json\" in path \"./config/config.json\"!")
		panic(fileErr)
	}

	jsonErr := json.Unmarshal(fileBytes, &config)
	if jsonErr != nil {
		log.Println("Unable to unmarshal file \"config.json\", are you sure the format is correct?")
		panic(jsonErr)
	}

	initResponses()
}

func main() {
	var connectErr error
	botSession, connectErr = discordgo.New("Bot " + config.BotToken)
	if connectErr != nil {
		panic(connectErr)
	}

	botSession.AddHandler(handleIncomingMessage)
	botSession.AddHandler(handleInteractions)

	botSession.Identify.Intents = discordgo.IntentsDirectMessages

	connectErr = botSession.Open()
	if connectErr != nil {
		panic(connectErr)
	}
	defer botSession.Close()

	_, slashCommandErr := botSession.ApplicationCommandCreate(config.ApplicationID, config.GuildID, &discordgo.ApplicationCommand{
		Name:        "bugreportbutton",
		Description: "Used to create a button for submitting a bug report",
	})
	if slashCommandErr != nil {
		log.Println("Unable to create the slash command!")
		panic(slashCommandErr)
	}

	log.Println("Bot is online!")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Graceful shutdown!")
}

func startNewReportConversation(userID string, interactionButtonChannelID string) (succeeded bool) {
	currentReportsMutex.Lock()
	defer currentReportsMutex.Unlock()

	// TODO add cooldown function in here.

	if isAlreadyInReportProcess(userID) {
		// TODO maybe provide a message that they are already making a report and can cancel it with a command?
		return
	}

	questions := make([]reportQuestionData, len(config.Questions))
	for index, question := range config.Questions {
		fixedFormats := make([]string, len(config.Questions))
		for fixedIndex, fixedAnswer := range question.FixedAnswers {
			fixedFormats[fixedIndex] = strings.ToLower(fixedAnswer)
		}

		questions[index] = reportQuestionData{
			question: reportQuestionFormatted{
				reportQuestion:        question,
				fixedAnswersFormatted: fixedFormats,
			},
		}
	}

	report := &reportData{
		currentQuestionIndex: 1,
		lastInteraction:      time.Now(),
		data:                 questions,
		lock:                 new(sync.Mutex),
	}

	if !sendFirstQuestion(report, userID) {
		sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID)
		return false
	}

	currentOngoingReports[userID] = report
	return true
}

// If we can't create a report and the channel ID on which a person possibly clicked isn't empty
// then we send some feedback that the user should open their DMs
func sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID string) {
	if interactionButtonChannelID != "" {
		go botSession.ChannelMessageSend(interactionButtonChannelID, strings.ReplaceAll(config.Messages.UnableToDMPerson, "{{USER_TAG}}", "<@"+userID+">"))
	}
}

// This method checks whether a user is already in an ongoing report process.
// WARNING! This one does not lock the mutex needed to access the data!
func isAlreadyInReportProcess(userID string) bool {
	_, ok := currentOngoingReports[userID]
	return ok
}

func sendFirstQuestion(report *reportData, userID string) (succeeded bool) {
	channel, channelErr := botSession.UserChannelCreate(userID)
	if channelErr != nil {
		return false
	}

	formattedFirstQuestion := strings.ReplaceAll(config.Messages.WelcomeMessage, "{{REPORT_TIMEOUT}}", strconv.Itoa(int(config.ReportTimeoutMinutes))) + "\n\n" + report.data[0].question.Question
	_, messageErr := botSession.ChannelMessageSend(channel.ID, formattedFirstQuestion)
	return messageErr == nil
}

type basicConfig struct {
	BotToken           string `json:"bot_token"`
	ApplicationID      string `json:"application_id"`
	GuildID            string `json:"guild_id"`
	BotDMCommandPrefix string `json:"bot_dm_command_prefix"`

	ReportChannelID      string           `json:"report_channel_id"`
	Questions            []reportQuestion `json:"questions"`
	ReportTimeoutMinutes uint             `json:"report_timeout_minutes"`

	Messages messagesDataConfig `json:"messages_data"`
}

type messagesDataConfig struct {
	InteractionNotAllowed    string `json:"interaction_not_allowed"`
	InteractionButtonContent string `json:"interaction_button_content"`
	UnableToDMPerson         string `json:"unable_to_dm_person"`
	WelcomeMessage           string `json:"welcome_message"`
}

type reportData struct {
	currentQuestionIndex uint
	lastInteraction      time.Time
	data                 []reportQuestionData
	lock                 *sync.Mutex
}

type reportQuestionData struct {
	answer   string
	question reportQuestionFormatted
}

type reportQuestion struct {
	Question     string   `json:"question"`
	PrettyFormat string   `json:"pretty_format"`
	FixedAnswers []string `json:"fixed_answers,omitempty"`
}

type reportQuestionFormatted struct {
	reportQuestion
	fixedAnswersFormatted []string
}
