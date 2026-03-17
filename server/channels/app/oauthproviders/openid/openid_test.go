// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthopenid

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistration(t *testing.T) {
	provider := einterfaces.GetOAuthProvider(model.ServiceOpenid)
	require.NotNil(t, provider, "OpenID provider should be registered")
	require.IsType(t, &OpenIDProvider{}, provider, "Provider should be OpenIDProvider")
}

func TestOpenIDUserFromJSON(t *testing.T) {
	rctx := request.TestContext(t)
	provider := &OpenIDProvider{}

	validUser := OpenIDUser{
		Sub:               "subject-123",
		PreferredUsername: "preferred.user",
		Name:              "Preferred User",
		GivenName:         "Preferred",
		FamilyName:        "User",
		Email:             "preferred@example.com",
		EmailVerified:     true,
	}

	t.Run("valid openid user with preferred username", func(t *testing.T) {
		b, err := json.Marshal(validUser)
		require.NoError(t, err)

		user, err := provider.GetUserFromJSON(rctx, bytes.NewReader(b), nil)
		require.NoError(t, err)

		require.NotNil(t, user)
		assert.Equal(t, "preferred.user", user.Username)
		assert.Equal(t, "preferred@example.com", user.Email)
		require.NotNil(t, user.AuthData)
		assert.Equal(t, validUser.Sub, *user.AuthData)
		assert.Equal(t, model.ServiceOpenid, user.AuthService)
	})

	t.Run("valid openid user without preferred username", func(t *testing.T) {
		noPreferred := validUser
		noPreferred.PreferredUsername = ""

		b, err := json.Marshal(noPreferred)
		require.NoError(t, err)

		user, err := provider.GetUserFromJSON(rctx, bytes.NewReader(b), nil)
		require.NoError(t, err)

		require.NotNil(t, user)
		assert.Equal(t, "preferred", user.Username)
	})

	t.Run("valid openid user default behavior", func(t *testing.T) {
		b, err := json.Marshal(validUser)
		require.NoError(t, err)

		user, err := provider.GetUserFromJSON(rctx, bytes.NewReader(b), nil)
		require.NoError(t, err)

		require.NotNil(t, user)
		assert.Equal(t, "preferred.user", user.Username)
	})

	t.Run("empty body should fail validation", func(t *testing.T) {
		_, err := provider.GetUserFromJSON(rctx, strings.NewReader("{}"), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user 'sub' claim is required")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := provider.GetUserFromJSON(rctx, strings.NewReader("invalid json"), nil)
		require.Error(t, err)
	})
}

func TestUserFromOpenIDUser(t *testing.T) {
	logger := mlog.CreateConsoleTestLogger(t)

	testCases := []struct {
		description       string
		oidcUser          OpenIDUser
		expectedUsername  string
		expectedFirstName string
		expectedLastName  string
		expectedEmail     string
		expectedAuthData  string
	}{
		{
			description: "Username from PreferredUsername when present",
			oidcUser: OpenIDUser{
				Sub:               "1",
				PreferredUsername: "preferred.user",
				GivenName:         "First",
				FamilyName:        "Last",
				Email:             "TEST@EXAMPLE.COM",
			},
			expectedUsername:  "preferred.user",
			expectedFirstName: "First",
			expectedLastName:  "Last",
			expectedEmail:     "test@example.com",
			expectedAuthData:  "1",
		},
		{
			description: "Username from Email when PreferredUsername is empty",
			oidcUser: OpenIDUser{
				Sub:        "2",
				GivenName:  "First",
				FamilyName: "Last",
				Email:      "some.user@example.com",
			},
			expectedUsername:  "some.user",
			expectedFirstName: "First",
			expectedLastName:  "Last",
			expectedEmail:     "some.user@example.com",
			expectedAuthData:  "2",
		},
		{
			description: "Username needing cleaning from preferred username",
			oidcUser: OpenIDUser{
				Sub:               "3",
				PreferredUsername: "preferred@@user!!",
				Name:              "Given Family",
				Email:             "user@example.com",
			},
			expectedUsername:  "preferred--user",
			expectedFirstName: "Given",
			expectedLastName:  "Family",
			expectedEmail:     "user@example.com",
			expectedAuthData:  "3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			user := userFromOpenIDUser(logger, &tc.oidcUser)

			require.NotNil(t, user)
			assert.Equal(t, tc.expectedUsername, user.Username)
			assert.Equal(t, tc.expectedFirstName, user.FirstName)
			assert.Equal(t, tc.expectedLastName, user.LastName)
			assert.Equal(t, tc.expectedEmail, user.Email)
			require.NotNil(t, user.AuthData)
			assert.Equal(t, tc.expectedAuthData, *user.AuthData)
			assert.Equal(t, model.ServiceOpenid, user.AuthService)
		})
	}
}
