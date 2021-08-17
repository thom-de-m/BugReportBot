package main

import (
	"strings"
	"sync"
	"time"
)

var (
	currentUsersOnReportCooldown = make(map[string]time.Time)
	currentUsersOnReportMutex    = new(sync.RWMutex)

	userCooldownsMessages      = make(map[string]time.Time)
	userCooldownsMessagesMutex = new(sync.RWMutex)
)

func setReportCooldownForUser(userID string) {
	currentUsersOnReportMutex.Lock()
	defer currentUsersOnReportMutex.Unlock()

	currentUsersOnReportCooldown[userID] = time.Now().Add(time.Duration(config.ReportCooldownMinutes) * time.Minute)
}

func isUserOnReportCooldown(userID string) bool {
	currentUsersOnReportMutex.RLock()
	defer currentUsersOnReportMutex.RUnlock()

	if cooldown, ok := currentUsersOnReportCooldown[userID]; ok {
		if time.Now().Before(cooldown) {
			return true
		}
	}
	return false
}

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

func isValidFixedQuestionAnswer(report *reportData, content string) bool {
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
