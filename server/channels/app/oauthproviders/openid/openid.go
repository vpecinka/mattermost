// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthopenid

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

type OpenIDProvider struct {
}

// OpenIDUser represents the user info from OpenID Connect /userinfo endpoint
type OpenIDUser struct {
	Sub               string `json:"sub"`                // Subject - unique identifier
	PreferredUsername string `json:"preferred_username"` // Username
	Name              string `json:"name"`               // Full name
	GivenName         string `json:"given_name"`         // First name
	FamilyName        string `json:"family_name"`        // Last name
	Email             string `json:"email"`              // Email
	EmailVerified     bool   `json:"email_verified"`     // Email verification status
}

func init() {
	provider := &OpenIDProvider{}
	einterfaces.RegisterOAuthProvider(model.ServiceOpenid, provider)
}

func userFromOpenIDUser(logger mlog.LoggerIFace, oidcUser *OpenIDUser) *model.User {
	user := &model.User{}

	// Use preferred_username if available, otherwise use email prefix
	username := oidcUser.PreferredUsername
	if username == "" && oidcUser.Email != "" {
		username = strings.Split(oidcUser.Email, "@")[0]
	}
	user.Username = model.CleanUsername(logger, username)

	// Set names from OIDC claims
	if oidcUser.GivenName != "" {
		user.FirstName = oidcUser.GivenName
	}
	if oidcUser.FamilyName != "" {
		user.LastName = oidcUser.FamilyName
	}
	// Fallback to splitting full name if given/family names not provided
	if user.FirstName == "" && user.LastName == "" && oidcUser.Name != "" {
		splitName := strings.Split(oidcUser.Name, " ")
		if len(splitName) == 2 {
			user.FirstName = splitName[0]
			user.LastName = splitName[1]
		} else if len(splitName) > 2 {
			user.FirstName = splitName[0]
			user.LastName = strings.Join(splitName[1:], " ")
		} else {
			user.FirstName = oidcUser.Name
		}
	}

	user.Email = oidcUser.Email
	user.Email = strings.ToLower(user.Email)

	// Use 'sub' claim as AuthData (unique identifier)
	userId := oidcUser.Sub
	user.AuthData = &userId
	user.AuthService = model.ServiceOpenid

	return user
}

func openIDUserFromJSON(data io.Reader) (*OpenIDUser, error) {
	decoder := json.NewDecoder(data)
	var oidcUser OpenIDUser
	err := decoder.Decode(&oidcUser)
	if err != nil {
		return nil, err
	}
	return &oidcUser, nil
}

func (oidcUser *OpenIDUser) IsValid() error {
	if oidcUser.Sub == "" {
		return errors.New("user 'sub' claim is required")
	}

	if oidcUser.Email == "" {
		return errors.New("user email should not be empty")
	}

	return nil
}

func (op *OpenIDProvider) GetUserFromJSON(rctx request.CTX, data io.Reader, tokenUser *model.User) (*model.User, error) {
	oidcUser, err := openIDUserFromJSON(data)
	if err != nil {
		return nil, err
	}
	if err = oidcUser.IsValid(); err != nil {
		return nil, err
	}

	return userFromOpenIDUser(rctx.Logger(), oidcUser), nil
}

func (op *OpenIDProvider) GetSSOSettings(_ request.CTX, config *model.Config, service string) (*model.SSOSettings, error) {
	return &config.OpenIdSettings, nil
}

func (op *OpenIDProvider) GetUserFromIdToken(_ request.CTX, idToken string) (*model.User, error) {
	return nil, nil
}

func (op *OpenIDProvider) IsSameUser(_ request.CTX, dbUser, oauthUser *model.User) bool {
	return dbUser.AuthData == oauthUser.AuthData
}
