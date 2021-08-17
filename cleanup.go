package main

import (
	"time"
)

func startCleanupTimer() {
	ticker := time.NewTicker(10 * time.Second)

	for {
		currentTime := <-ticker.C
		go checkOnGoingMessagesCooldown(currentTime)
		go checkOngoingReportCooldowns(currentTime)
		go checkOngoingReportCleanup(currentTime)
	}
}

func removeNeededCooldownUsers(currentTime time.Time, mapPointer map[string]time.Time) {
	markedForRemoval := make([]string, 0)

	for userID, cooldown := range mapPointer {
		if currentTime.After(cooldown) {
			markedForRemoval = append(markedForRemoval, userID)
		}
	}

	for _, userID := range markedForRemoval {
		delete(mapPointer, userID)
	}
}

func checkOnGoingMessagesCooldown(currentTime time.Time) {
	userCooldownsMessagesMutex.Lock()
	defer userCooldownsMessagesMutex.Unlock()

	removeNeededCooldownUsers(currentTime, userCooldownsMessages)
}

func checkOngoingReportCooldowns(currentTime time.Time) {
	currentUsersOnReportMutex.Lock()
	defer currentUsersOnReportMutex.Unlock()

	removeNeededCooldownUsers(currentTime, currentUsersOnReportCooldown)
}

func checkOngoingReportCleanup(currentTime time.Time) {
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
