package common

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	UserIndex    = "users"
	ChannelIndex = "channels"
)

// ESChannel represents a channel document in ElasticSearch
type ESChannel struct {
	Id            string            `json:"id"`
	Type          model.ChannelType `json:"type"`
	DeleteAt      int64             `json:"delete_at"`
	UserIDs       []string          `json:"user_ids"`
	TeamId        string            `json:"team_id"`
	TeamMemberIDs []string          `json:"team_member_ids"`
	NameSuggest   []string          `json:"name_suggestions"`
}

// ESUser represents a user document in ElasticSearch
type ESUser struct {
	Id                         string   `json:"id"`
	SuggestionsWithFullname    []string `json:"suggestions_with_fullname"`
	SuggestionsWithoutFullname []string `json:"suggestions_without_fullname"`
	DeleteAt                   int64    `json:"delete_at"`
	Roles                      []string `json:"roles"`
	TeamsIds                   []string `json:"team_id"`
	ChannelsIds                []string `json:"channel_id"`
}

// GetSuggestionInputsSplitBy splits a string by a delimiter and returns all prefixes
// Example: "john.doe" with separator "." returns ["john", "john.doe", "doe"]
func GetSuggestionInputsSplitBy(input string, separator string) []string {
	if input == "" {
		return []string{}
	}

	input = strings.ToLower(input)
	parts := strings.Split(input, separator)

	suggestions := make([]string, 0, len(parts)+1)

	// Add full input
	suggestions = append(suggestions, input)

	// Add each part
	for _, part := range parts {
		if part != "" && part != input {
			suggestions = append(suggestions, part)
		}
	}

	return removeDuplicates(suggestions)
}

// GetSuggestionInputsSplitByMultiple splits a string by multiple delimiters
func GetSuggestionInputsSplitByMultiple(input string, separators []string) []string {
	if input == "" {
		return []string{}
	}

	suggestions := make([]string, 0)
	suggestions = append(suggestions, strings.ToLower(input))

	currentParts := []string{input}

	for _, separator := range separators {
		newParts := make([]string, 0)
		for _, part := range currentParts {
			splitParts := strings.Split(part, separator)
			for _, splitPart := range splitParts {
				if splitPart != "" {
					newParts = append(newParts, splitPart)
					suggestions = append(suggestions, strings.ToLower(splitPart))
				}
			}
		}
		currentParts = newParts
	}

	return removeDuplicates(suggestions)
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// ESUserFromUserAndTeams creates an ESUser from a User and team/channel IDs
func ESUserFromUserAndTeams(user *model.User, teamsIds, channelsIds []string) *ESUser {
	usernameSuggestions := GetSuggestionInputsSplitByMultiple(user.Username, []string{".", "-", "_"})

	fullnameStrings := []string{}
	if user.FirstName != "" {
		fullnameStrings = append(fullnameStrings, user.FirstName)
	}
	if user.LastName != "" {
		fullnameStrings = append(fullnameStrings, user.LastName)
	}

	fullnameSuggestions := []string{}
	if len(fullnameStrings) > 0 {
		fullname := strings.Join(fullnameStrings, " ")
		fullnameSuggestions = GetSuggestionInputsSplitBy(fullname, " ")
	}

	nicknameSuggestions := []string{}
	if user.Nickname != "" {
		nicknameSuggestions = GetSuggestionInputsSplitBy(user.Nickname, " ")
	}

	usernameAndNicknameSuggestions := append(usernameSuggestions, nicknameSuggestions...)

	return &ESUser{
		Id:                         user.Id,
		SuggestionsWithFullname:    append(usernameAndNicknameSuggestions, fullnameSuggestions...),
		SuggestionsWithoutFullname: usernameAndNicknameSuggestions,
		DeleteAt:                   user.DeleteAt,
		Roles:                      user.GetRoles(),
		TeamsIds:                   teamsIds,
		ChannelsIds:                channelsIds,
	}
}

// ESUserFromUserForIndexing creates an ESUser from UserForIndexing
func ESUserFromUserForIndexing(userForIndexing *model.UserForIndexing) *ESUser {
	user := &model.User{
		Id:        userForIndexing.Id,
		Username:  userForIndexing.Username,
		Nickname:  userForIndexing.Nickname,
		FirstName: userForIndexing.FirstName,
		Roles:     userForIndexing.Roles,
		LastName:  userForIndexing.LastName,
		CreateAt:  userForIndexing.CreateAt,
		DeleteAt:  userForIndexing.DeleteAt,
	}

	return ESUserFromUserAndTeams(user, userForIndexing.TeamsIds, userForIndexing.ChannelsIds)
}

// ESChannelFromChannel creates an ESChannel from a Channel
func ESChannelFromChannel(channel *model.Channel, userIDs, teamMemberIDs []string) *ESChannel {
	displayNameInputs := GetSuggestionInputsSplitBy(channel.DisplayName, " ")
	nameInputs := GetSuggestionInputsSplitByMultiple(channel.Name, []string{"-", "_"})

	return &ESChannel{
		Id:            channel.Id,
		Type:          channel.Type,
		DeleteAt:      channel.DeleteAt,
		UserIDs:       userIDs,
		TeamId:        channel.TeamId,
		TeamMemberIDs: teamMemberIDs,
		NameSuggest:   append(displayNameInputs, nameInputs...),
	}
}
