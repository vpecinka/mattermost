#!/bin/zsh

export BUILD_NUMBER=szn-v11.0.2 

# Plugins to pre-package
PLUGIN_PACKAGES="mattermost-plugin-jira-v4.3.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-gitlab-v1.10.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-zoom-v1.8.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-boards-v9.1.6"
export PLUGIN_PACKAGES

echo "Building BUILD_NUMBER=${BUILD_NUMBER}"
echo "Plugins: ${PLUGIN_PACKAGES}"
echo "Any key to continue, CTRL+C to break..."
read


# NOTE: How to update repo
#
# git fetch upstream        # or git fetch origin, as you wish
# git checkout szn-patch
# git rebase v11.1.0        # or a tag of your choice
#
# If there are conflicts
#    - edit respective files
#    - git add <file>
#    - git rebase --continue
#



echo "\n===== BUILDING WEBAPP ================\n"
cd webapp
. ~/.nvm/nvm.sh
make dist

echo "\n===== BUILDING SERVER ================\n"
cd ../server
make build-linux-amd64 


echo "\n===== MAKING APP PACKAGE =============\n"
make package-linux-amd64

cd ..

echo "\n===== TESTING APP PACKAGE ============\n"
echo "Testing package server/dist/mattermost-team-linux-amd64.tar.gz"
tar tzf server/dist/mattermost-team-linux-amd64.tar.gz >/dev/null && echo "Package build sucessfuly"
