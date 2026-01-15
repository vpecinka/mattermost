// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.
//
// This implementation allows enabling enterprise features that are either
// fully implemented in open source code or have custom Seznam.cz implementations,
// without requiring a paid Mattermost license.

package custom

import "github.com/mattermost/mattermost/server/public/model"

// GetCustomLicense returns a mock Professional license with Seznam.cz custom features enabled.
// This allows using enterprise features that are:
// - Fully implemented in open source code
// - Have custom Seznam.cz implementations in server/custom/
//
// When no official license is installed, this license is automatically used to enable
// the custom implementations without requiring a paid Mattermost license.
//
// To enable a new feature when you implement it:
// 1. Add the feature name to the list below (see available features in comments)
// 2. Restart the server
//
// Available features (based on model.Features):
// - "ldap_groups"          Custom/LDAP groups
// - "elastic_search"       Elasticsearch integration
// - "cluster"              High availability clustering
// - "metrics"              Prometheus metrics
// - "data_retention"       Data retention policies / Scheduled posts
// - "compliance"           Compliance reporting
// - "saml"                 SAML SSO
// - "mfa"                  Multi-factor authentication
// - "guest_accounts"       Guest user support
// - "custom_terms_of_service"  Custom Terms of Service
// - "message_export"       Message export
// - "advanced_logging"     Advanced logging
// - "enterprise_plugins"   Enterprise plugins
// - "shared_channels"      Shared channels
// And more... (see server/public/model/license.go Features struct)
func GetCustomLicense() *model.License {
	lic := model.NewTestLicenseSKU(
		model.LicenseShortSkuProfessional,
		// Seznam.cz custom implementations - enabled features:
		"ldap_groups",    // Custom groups support (CUSTOM_USER_GROUPS.md)
		"elastic_search", // Custom search implementation (sznsearch/)
		"metrics",        // Custom metrics (sznmetrics/)
		"cluster",        // Custom clustering (szncluster/)
		"data_retention", // Scheduled posts support (SCHEDULED_POSTS.md)
		// Add more features here as you implement them:
		// "compliance",
		// "saml",
		// "mfa",
		// "message_export",
		// etc.
	)

	// Set maximum number of users/seats to 20000
	maxUsers := 20000
	lic.Features.Users = &maxUsers

	return lic
}
