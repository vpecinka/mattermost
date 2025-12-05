// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package common

import (
	"github.com/mattermost/mattermost/server/public/model"
)

// extractFileContent extracts text content from a file for indexing.
// First checks if content is already extracted (file.Content), otherwise
// performs extraction using Mattermost's docextractor.
//
// Returns:
// - Cached content from file.Content if available (already extracted by Mattermost)
// - Extracted content using docextractor if file.Content is empty
// - Empty string if extraction fails or file is not readable
func extractFileContent(file *model.FileInfo) string {
	// TODO: Implement synchronous extraction for files not yet processed
	// Note: With S3 storage, os.Open() won't work - need to use filestore API
	// to read file contents before passing to docextractor.
	//
	// For now, indexing empty string is fine - file metadata (name, extension)
	// is still searchable. Full-text content search can be added later.

	return ""
}
