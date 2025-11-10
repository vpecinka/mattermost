#!/bin/zsh

export BUILD_NUMBER="szn-$(date +%y%m%d%H%M)"

# Plugins to pre-package
PLUGIN_PACKAGES="mattermost-plugin-jira-v4.4.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-gitlab-v1.11.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-zoom-v1.8.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-boards-v9.1.7"
export PLUGIN_PACKAGES

echo "Building BUILD_NUMBER=${BUILD_NUMBER}"
echo "Plugins: ${PLUGIN_PACKAGES}"
echo "Any key to continue, CTRL+C to break..."
read

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
