// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package oauthopenid

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistration(t *testing.T) {
	provider := einterfaces.GetOAuthProvider(model.ServiceOpenid)
	require.NotNil(t, provider, "OpenID provider should be registered")
	require.IsType(t, &OpenIDProvider{}, provider, "Provider should be OpenIDProvider")
}
