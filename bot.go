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
}

func main() {
	var connectErr error
	botSession, connectErr = discordgo.New("Bot " + config.BotToken)
	if connectErr != nil {
		panic(connectErr)
	}

	botSession.AddHandler(handleIncomingMessage)
	botSession.AddHandler(handleInteractions)

	botSession.Identify.Intents = discordgo.IntentsDirectMessages | discordgo.IntentsGuildMessages

	connectErr = botSession.Open()
	if connectErr != nil {
		panic(connectErr)
	}
	defer botSession.Close()

	log.Println("Bot is online!")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Graceful shutdown!")
}

func continueOngoingReport(report *reportData, content, userID string) {
	lowerCaseContent := strings.ToLower(content)
	if report.canSubmit && lowerCaseContent == config.BotDMCommandPrefix+config.BotDMCommandSubmit {
		// Someone wants to submit their report, lets do it!
		handleFinalSubmission(report, userID)
		return
	}

	if lowerCaseContent == config.BotDMCommandPrefix+config.BotDMCommandCancel {
		// Someone wants to cancel their report
		deleteOngoingReport(userID)
		return
	}

	// TODO add attachments check

	if !isValidAnswer(report, content) {
		baseFormat := config.Messages.InvalidFixedQuestionAnswer
		for _, value := range report.data[report.currentQuestionIndex].question.FixedAnswers {
			baseFormat += "\n- " + value
		}

		sendMessageToDM(baseFormat, userID)
		return
	}

	if !report.hasReachedEnd {
		report.data[report.currentQuestionIndex].answer = strings.ReplaceAll(content, "@", "at")
	}

	// If this validates true that means we are at the end of the report!
	if report.currentQuestionIndex+1 == uint(len(report.data)) {
		handleSubmittingProcess(report, userID)
		return
	}

	report.currentQuestionIndex += 1
	sendReportQuestion(report, userID, false)
}

func handleFinalSubmission(report *reportData, userID string) {
	botSession.ChannelMessageSend(config.ReportChannelID, generateFinalBugReport(report, false))
	removeReportAndUserFromCache(userID)
}

func handleSubmittingProcess(report *reportData, userID string) {
	// TODO check if the report isn't too big for a message!

	report.canEdit = true
	report.canSubmit = true
	report.hasReachedEnd = true

	baseString := config.Messages.FinalReportSubmitAlmostReady
	baseString = strings.ReplaceAll(baseString, "{{SUBMIT_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandSubmit)
	baseString = strings.ReplaceAll(baseString, "{{EDIT_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandEdit)

	sendMessageToDM(baseString, userID)
	sendMessageToDM(generateFinalBugReport(report, true), userID)
}

func deleteOngoingReport(userID string) {
	sendMessageToDM(config.Messages.CancellingReport, userID)
	removeReportAndUserFromCache(userID)
}

func removeReportAndUserFromCache(userID string) {
	currentReportsMutex.Lock()
	defer currentReportsMutex.Unlock()

	delete(currentOngoingReports, userID)
}

func generateFinalBugReport(report *reportData, highlightQuestionNumber bool) string {
	var builder strings.Builder
	for index, value := range report.data {
		if highlightQuestionNumber {
			builder.WriteString("**#")
			builder.WriteString(strconv.Itoa(index + 1))
			builder.WriteString("** ")
		}
		builder.WriteString(value.question.PrettyFormat)
		builder.WriteString("\n")
		builder.WriteString(value.answer)
		builder.WriteString("\n\n")
	}

	// TODO add attachments, the user who submitted it and possibly a last message.

	return builder.String()
}

func isValidAnswer(report *reportData, content string) bool {
	data := report.data[report.currentQuestionIndex]
	if len(data.question.FixedAnswers) > 0 {
		formattedContent := strings.ToLower(content)
		validAnswer := false

		for _, value := range data.question.fixedAnswersFormatted {
			if value == formattedContent {
				validAnswer = true
				break
			}
		}

		return validAnswer
	}

	return true
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
		fixedFormats := make([]string, len(config.Questions[index].FixedAnswers))
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
		currentQuestionIndex: 0,
		lastInteraction:      time.Now(),
		data:                 questions,
		lock:                 new(sync.Mutex),
		canEdit:              false,
		canSubmit:            false,
		hasReachedEnd:        false,
	}

	if !sendReportQuestion(report, userID, true) {
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

func sendReportQuestion(report *reportData, userID string, firstMessage bool) (succeeded bool) {
	formattedFirstQuestion := ""

	if firstMessage {
		formattedFirstQuestion = strings.ReplaceAll(config.Messages.WelcomeMessage, "{{REPORT_TIMEOUT}}", strconv.Itoa(int(config.ReportTimeoutMinutes))) + "\n\n"
	}

	formattedFirstQuestion += report.data[report.currentQuestionIndex].question.Question
	return sendMessageToDM(formattedFirstQuestion, userID)
}

func sendMessageToDM(content, userID string) (succeeded bool) {
	channel, channelErr := getUserChannel(userID)
	if channelErr != nil {
		return false
	}

	_, messageErr := botSession.ChannelMessageSend(channel.ID, content)
	return messageErr == nil
}

func getUserChannel(userID string) (channel *discordgo.Channel, err error) {
	return botSession.UserChannelCreate(userID)
}

type basicConfig struct {
	BotToken string `json:"bot_token"`
	GuildID  string `json:"guild_id"`

	BotDMCommandPrefix string `json:"bot_dm_command_prefix"`
	BotDMCommandSubmit string `json:"bot_dm_command_submit"`
	BotDMCommandEdit   string `json:"bot_dm_command_edit"`
	BotDMCommandCancel string `json:"bot_dm_command_cancel"`

	SubmitReportChannelID string           `json:"submit_report_channel_id"`
	ReportChannelID       string           `json:"report_channel_id"`
	Questions             []reportQuestion `json:"questions"`
	ReportTimeoutMinutes  uint             `json:"report_timeout_minutes"`

	Messages messagesDataConfig `json:"messages_data"`
}

type messagesDataConfig struct {
	CancellingReport             string `json:"cancelling_report"`
	FinalReportSubmitAlmostReady string `json:"final_report_submit_almost_ready"`
	InvalidFixedQuestionAnswer   string `json:"invalid_answer_to_question"`
	InteractionNotAllowed        string `json:"interaction_not_allowed"`
	InteractionButtonContent     string `json:"interaction_button_content"`
	UnableToDMPerson             string `json:"unable_to_dm_person"`
	WelcomeMessage               string `json:"welcome_message"`
}

type reportData struct {
	currentQuestionIndex uint
	lastInteraction      time.Time
	data                 []reportQuestionData
	lock                 *sync.Mutex
	canSubmit            bool
	canEdit              bool
	hasReachedEnd        bool
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
