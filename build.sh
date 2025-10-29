#!/bin/bash

export BUILD_NUMBER=szn-v11.0.2 


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
make dist

echo "\n===== BUILDING SERVER ================\n"
cd ../server
make build-linux-amd64 


echo "\n===== MAKING APP PACKAGE =============\n"
make package-linux-amd64

cd ..

echo "\n===== TESTING APP PACKAGE ============\n"
tar tzf server/dist/mattermost-team-linux-amd64.tar.gz
