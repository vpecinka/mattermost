#!/bin/bash

# Rebase script for szn-build branch
# Follows the rebase strategy defined in REBASE_STRATEGY.md

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Rebase Script for szn-build ===${NC}"

# Step 1: Checkout szn-build
echo -e "\n${YELLOW}Step 1: Checking out szn-build branch...${NC}"
if git checkout szn-build; then
    echo -e "${GREEN}✓ Successfully checked out szn-build${NC}"
else
    echo -e "${RED}✗ Failed to checkout szn-build${NC}"
    exit 1
fi

# Step 2: Fetch upstream
echo -e "\n${YELLOW}Step 2: Fetching upstream...${NC}"
if git fetch upstream; then
    echo -e "${GREEN}✓ Successfully fetched upstream${NC}"
else
    echo -e "${RED}✗ Failed to fetch upstream${NC}"
    exit 1
fi

# Step 3: Rebase onto upstream/master
echo -e "\n${YELLOW}Step 3: Rebasing onto upstream/master...${NC}"
if git rebase upstream/master; then
    echo -e "${GREEN}✓ Successfully rebased onto upstream/master${NC}"
else
    echo -e "${RED}✗ Rebase failed with conflicts${NC}"
    echo -e "${YELLOW}Please resolve conflicts and complete the rebase:${NC}"
    echo -e "  1. Resolve conflicts in the affected files"
    echo -e "  2. Run: git add <resolved-files>"
    echo -e "  3. Run: git rebase --continue"
    echo -e "  4. After rebase is complete, don't forget to run:"
    echo -e "     ${GREEN}git push origin szn-build --force-with-lease${NC}"
    exit 1
fi

# Step 4: Check if repo is clean
echo -e "\n${YELLOW}Step 4: Checking if repository is clean...${NC}"
if [[ -z $(git status --porcelain) ]]; then
    echo -e "${GREEN}✓ Repository is clean${NC}"
    
    # Step 5: Push to origin
    echo -e "\n${YELLOW}Step 5: Pushing to origin/szn-build...${NC}"
    if git push origin szn-build --force-with-lease; then
        echo -e "${GREEN}✓ Successfully pushed to origin/szn-build${NC}"
        echo -e "\n${GREEN}=== Rebase completed successfully! ===${NC}"
    else
        echo -e "${RED}✗ Failed to push to origin${NC}"
        echo -e "${YELLOW}Please push manually:${NC}"
        echo -e "  ${GREEN}git push origin szn-build --force-with-lease${NC}"
        exit 1
    fi
else
    echo -e "${RED}✗ Repository has uncommitted changes${NC}"
    echo -e "${YELLOW}Please commit or stash your changes, then run:${NC}"
    echo -e "  ${GREEN}git push origin szn-build --force-with-lease${NC}"
    git status --short
    exit 1
fi
