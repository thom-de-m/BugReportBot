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

	go startCleanupTimer()

	log.Println("Bot is online!")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Graceful shutdown!")
}

func startCleanupTimer() {
	ticker := time.NewTicker(10 * time.Second)

	for {
		currentTime := <-ticker.C
		doOngoingReportCleanup(currentTime)
	}
}

func doOngoingReportCleanup(currentTime time.Time) {
	currentReportsMutex.Lock()
	defer currentReportsMutex.Unlock()

	markedForRemoval := make([]string, 0)

	for userID, report := range currentOngoingReports {
		// If this validates true that means the last interaction with the user has been larger than our timeout
		if currentTime.After(report.lastInteraction.Add(time.Duration(config.ReportTimeoutMinutes) * time.Minute)) {
			markedForRemoval = append(markedForRemoval, userID)
		}
	}

	for _, userID := range markedForRemoval {
		delete(currentOngoingReports, userID)
		sendMessageToDM(config.Messages.InactiveReport, userID)
	}
}

func markReportAsActive(report *reportData) {
	report.lastInteraction = time.Now()
}

func continueOngoingReport(report *reportData, content, userID string, message *discordgo.MessageCreate) {
	// Handle attachements, if this returns true there was at least 1 attachment found
	if handleAttachments(report, userID, message) {
		if report.isInSubmitMenu {
			handleSubmittingProcess(report, userID)
		}
		return
	}

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

	if report.canEdit && strings.Split(lowerCaseContent, " ")[0] == config.BotDMCommandPrefix+config.BotDMCommandEdit {
		// Someone wants to edit a specific question
		handleEditReport(report, userID, content)
		return
	}

	if !isValidFixedQuestionAnswer(report, content) {
		baseFormat := config.Messages.InvalidFixedQuestionAnswer
		for _, value := range report.data[report.currentQuestionIndex].question.FixedAnswers {
			baseFormat += "\n- " + value
		}

		sendMessageToDM(baseFormat, userID)
		return
	}

	// From here on it's always a valid response, spam bots will get stuck on fixed questions and will eventually timeout
	// while regular users will most likely never be that long stuck on one single question
	markReportAsActive(report)

	if report.shouldReadAnswer {
		report.data[report.currentQuestionIndex].answer = strings.ReplaceAll(content, "@", "at")
	}

	// If this validates true that means we are at the end of the report!
	if report.currentQuestionIndex+1 == uint(len(report.data)) || report.hasReachedEnd {
		if report.hasReachedEnd {
			report.currentQuestionIndex = uint(len(report.data)) - 1
		}
		handleSubmittingProcess(report, userID)
		return
	}

	report.currentQuestionIndex += 1
	sendReportQuestion(report, userID, false)
}

func handleAttachments(report *reportData, userID string, message *discordgo.MessageCreate) (attachedAttachements bool) {
	if len(message.Attachments) == 0 {
		return false
	}

	affectedItems := 0
	for _, attachment := range message.Attachments {
		if uint(len(report.attachments)) >= config.ReportMaxAttachments {
			break
		}

		report.attachments = append(report.attachments, attachment.ProxyURL)
		affectedItems += 1
	}

	if affectedItems == 0 {
		sendMessageToDM(config.Messages.ReachedMaxAttachments, userID)
		return true
	}

	var baseString string
	if affectedItems > 1 {
		baseString = config.Messages.AttachmentUploadedPlural

	} else {
		baseString = config.Messages.AttachmentUploaded
	}

	baseString = strings.ReplaceAll(baseString, "{{ATTACHMENTS_LEFT}}", strconv.Itoa(int(config.ReportMaxAttachments)-len(report.attachments)))
	sendMessageToDM(baseString, userID)

	return true
}

func handleEditReport(report *reportData, userID, content string) {
	split := strings.Split(content, " ")
	if len(split) != 2 {
		sendMessageToDM(config.Messages.ValidNumber, userID)
		return
	}

	value, parseErr := strconv.Atoi(split[1])
	if parseErr != nil {
		sendMessageToDM(config.Messages.ValidNumber, userID)
		return
	}

	if value <= 0 || value > len(report.data) {
		sendMessageToDM(config.Messages.ValidReportNumber, userID)
		return
	}

	report.isInSubmitMenu = false
	report.shouldReadAnswer = true
	report.canEdit = false
	report.currentQuestionIndex = uint(value) - 1
	sendReportQuestion(report, userID, false)
}

func handleFinalSubmission(report *reportData, userID string) {
	botSession.ChannelMessageSend(config.ReportChannelID, generateFinalBugReport(report, false, userID))

	// Invalidate the report
	report.canEdit = false
	report.canSubmit = false
	report.shouldReadAnswer = false
	report.isInSubmitMenu = false

	// Set report cooldown
	setReportCooldownForUser(userID)

	// Remove from cache
	removeReportAndUserFromCache(userID)

	baseString := config.Messages.SuccessfullySubmittedReport
	baseString = strings.ReplaceAll(baseString, "{{REPORT_COOLDOWN}}", strconv.Itoa(int(config.ReportCooldownMinutes)))
	sendMessageToDM(baseString, userID)
}

func handleSubmittingProcess(report *reportData, userID string) {
	// TODO check if the report isn't too big for a message!

	report.canEdit = true
	report.canSubmit = true
	report.hasReachedEnd = true
	report.shouldReadAnswer = false
	report.isInSubmitMenu = true

	baseString := config.Messages.FinalReportSubmitAlmostReady
	baseString = strings.ReplaceAll(baseString, "{{CANCEL_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandCancel)
	baseString = strings.ReplaceAll(baseString, "{{SUBMIT_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandSubmit)
	baseString = strings.ReplaceAll(baseString, "{{EDIT_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandEdit)

	sendMessageToDM(baseString, userID)
	sendMessageToDM(generateFinalBugReport(report, true, userID), userID)
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

func generateFinalBugReport(report *reportData, highlightQuestionNumber bool, userID string) string {
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
		if index != len(report.data)-1 {
			builder.WriteString("\n\n")
		}
	}

	if len(report.attachments) > 0 {
		builder.WriteString(config.Messages.Attachments)
		for _, attachmentLink := range report.attachments {
			builder.WriteString("\n")
			builder.WriteString(attachmentLink)
		}
	}

	builder.WriteString(strings.ReplaceAll(config.Messages.EndMessageReport, "{{USER_TAG}}", "<@"+userID+">"))

	return builder.String()
}

func startNewReportConversation(userID string, interactionButtonChannelID string) {
	currentReportsMutex.Lock()
	defer currentReportsMutex.Unlock()

	if isAlreadyInReportProcess(userID) {
		// Adding the cooldown so the user can't spam! It's not completely fool proof due to multithreading, but that doesn't really matter
		if setAndCheckCooldownForUserMessages(userID) {
			return
		}

		baseString := config.Messages.AlreadyCreatingReport
		baseString = strings.ReplaceAll(baseString, "((CANCEL_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandCancel)
		if !sendMessageToDM(baseString, userID) {
			sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID)
		}
		return
	}

	if isUserOnReportCooldown(userID) {
		if setAndCheckCooldownForUserMessages(userID) {
			return
		}

		if !sendMessageToDM(config.Messages.ReportCooldown, userID) {
			sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID)
		}
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
		attachments:          make([]string, 0),
		currentQuestionIndex: 0,
		lastInteraction:      time.Now(),
		data:                 questions,
		lock:                 new(sync.Mutex),
		canEdit:              false,
		canSubmit:            false,
		hasReachedEnd:        false,
		shouldReadAnswer:     true,
		isInSubmitMenu:       false,
	}

	if !sendReportQuestion(report, userID, true) {
		// Set the user on a cooldown
		if setAndCheckCooldownForUserMessages(userID) {
			return
		}

		sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID)
		return
	}

	currentOngoingReports[userID] = report
}

// If we can't create a report and the channel ID on which a person possibly clicked isn't empty
// then we send some feedback that the user should open their DMs
func sendDMFailedMessageIfNeeded(userID, interactionButtonChannelID string) {
	if interactionButtonChannelID != "" {
		go func() {
			message, messageErr := botSession.ChannelMessageSend(interactionButtonChannelID, strings.ReplaceAll(config.Messages.UnableToDMPerson, "{{USER_TAG}}", "<@"+userID+">"))
			if messageErr != nil {
				return
			}

			timer := time.NewTimer(time.Duration(config.RemoveButtonMessagesAfterSeconds) * time.Second)
			<-timer.C

			botSession.ChannelMessageDelete(message.ChannelID, message.ID)
		}()
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
		formattedFirstQuestion = strings.ReplaceAll(formattedFirstQuestion, "{{CANCEL_COMMAND}}", config.BotDMCommandPrefix+config.BotDMCommandCancel)
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

	BotDMCommandPrefix string `json:"bot_dm_command_prefix"`
	BotDMCommandSubmit string `json:"bot_dm_command_submit"`
	BotDMCommandEdit   string `json:"bot_dm_command_edit"`
	BotDMCommandCancel string `json:"bot_dm_command_cancel"`

	SubmitReportChannelID            string           `json:"submit_report_channel_id"`
	ReportChannelID                  string           `json:"report_channel_id"`
	Questions                        []reportQuestion `json:"questions"`
	ReportTimeoutMinutes             uint             `json:"report_timeout_minutes"`
	ReportMaxAttachments             uint             `json:"report_max_attachments"`
	RemoveButtonMessagesAfterSeconds uint             `json:"remove_button_messages_after_seconds"`
	ReportMessagesCooldownSeconds    uint             `json:"report_messages_cooldown_seconds"`
	ReportCooldownMinutes            uint             `json:"report_cooldown_minutes"`

	Messages messagesDataConfig `json:"messages_data"`
}

type messagesDataConfig struct {
	ReportCooldown               string `json:"report_cooldown"`
	InactiveReport               string `json:"report_timeout"`
	AlreadyCreatingReport        string `json:"already_creating_report"`
	SuccessfullySubmittedReport  string `json:"thanks_for_submitting_a_report"`
	ReachedMaxAttachments        string `json:"reached_max_attachments"`
	Attachments                  string `json:"attachments"`
	AttachmentUploaded           string `json:"attachment_uploaded_with_report"`
	AttachmentUploadedPlural     string `json:"attachment_uploaded_with_report_plural"`
	EndMessageReport             string `json:"end_message_report"`
	ValidReportNumber            string `json:"valid_report_number"`
	ValidNumber                  string `json:"valid_number"`
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
	attachments          []string
	lock                 *sync.Mutex

	isInSubmitMenu   bool
	canSubmit        bool
	canEdit          bool
	hasReachedEnd    bool
	shouldReadAnswer bool
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
