#!/bin/zsh

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

export BUILD_NUMBER="szn-$(date +%y%m%d-%H%M)"

# Plugins to pre-package
PLUGIN_PACKAGES="mattermost-plugin-jira-v4.4.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-gitlab-v1.11.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-zoom-v1.8.0"
PLUGIN_PACKAGES="$PLUGIN_PACKAGES mattermost-plugin-boards-v9.1.7"
export PLUGIN_PACKAGES

echo -e "${GREEN}Building BUILD_NUMBER=${BUILD_NUMBER}${NC}"
echo -e "${BLUE}Plugins: ${PLUGIN_PACKAGES}${NC}"

echo -e "\n${YELLOW}===== BUILDING WEBAPP ================${NC}\n"
cd webapp
. ~/.nvm/nvm.sh
make dist

echo -e "\n${YELLOW}===== BUILDING SERVER ================${NC}\n"
cd ../server
make build-linux-amd64 


echo -e "\n${YELLOW}===== MAKING APP PACKAGE =============${NC}\n"
make package-linux-amd64

cd ..

echo -e "\n${YELLOW}===== TESTING APP PACKAGE ============${NC}\n"
echo -e "Testing package ${BLUE}server/dist/mattermost-team-linux-amd64.tar.gz${NC}"
tar tzf server/dist/mattermost-team-linux-amd64.tar.gz >/dev/null && echo -e "${GREEN}✓ Package build successfully${NC}"
