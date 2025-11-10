# Rebase Strategy for Custom Mattermost Build

## Branch Structure

### `szn-build` (main development branch)
- **Purpose**: Active development branch for custom changes
- **Based on**: `upstream/master` (always tracking the latest upstream main branch)
- **Usage**: 
  - All new custom changes are committed here
  - Regular rebase to keep up-to-date with upstream
  - Source for production builds

### `szn-prod-vX.Y.Z` (production release branches)
- **Purpose**: Branches for specific production releases
- **Based on**: Specific upstream tags (e.g., `v11.1.0`, `v11.2.0`)
- **Usage**: Cherry-pick all changes from `szn-build` for production deployment
- **Example**: `szn-prod-v11.1.0`, `szn-prod-v11.2.0`

---

## Daily Workflow

### Working on new features
```bash
# Switch to development branch
git checkout szn-build

# Make sure you have latest upstream changes
git fetch upstream
git rebase upstream/master

# Create your changes
# ... edit files ...

# Commit changes
git add .
git commit -m "feat: description of change"

# Push to your fork
git push origin szn-build --force-with-lease
```

### Regular upstream sync (weekly/monthly)
```bash
# Switch to development branch
git checkout szn-build

# Fetch latest upstream changes
git fetch upstream

# Rebase your changes on top of upstream
git rebase upstream/master

# Force push (safely) to your fork
git push origin szn-build --force-with-lease
```

**Note**: Use `--force-with-lease` instead of `--force` to prevent accidentally overwriting work if someone else pushed to the branch.

---

## Production Release Workflow

### Creating a production build for a new release

When a new upstream release is tagged (e.g., `v11.2.0`), create a production branch:

```bash
# 1. Update your development branch first
git checkout szn-build
git fetch upstream
git rebase upstream/master
git push origin szn-build --force-with-lease

# 2. Create production branch from the release tag
git checkout -b szn-prod-v11.2.0 v11.2.0

# 3. Cherry-pick ALL your custom changes at once
git cherry-pick upstream/master..szn-build

# 4. Push production branch
git push origin szn-prod-v11.2.0
```

### If cherry-pick conflicts occur

```bash
# Resolve conflicts in the conflicted files
# ... edit files ...

# Stage resolved files
git add <resolved-files>

# Continue cherry-pick
git cherry-pick --continue

# If you need to abort and start over
git cherry-pick --abort
```

---

## Common Commands Reference

### Check current branch status
```bash
# Show current branch
git branch --show-current

# Show branches and their tracking
git branch -vv
```

### View your custom commits
```bash
# List all commits in szn-build not in upstream/master
git log --oneline upstream/master..szn-build

# Count your custom commits
git log --oneline upstream/master..szn-build | wc -l

# Show detailed diff
git diff upstream/master..szn-build
```

### Fetch and check upstream updates
```bash
# Fetch all upstream branches and tags
git fetch upstream

# Check if there are new upstream commits
git log --oneline szn-build..upstream/master

# View what changed in upstream
git log --oneline --graph upstream/master -10
```

### Emergency: Undo a rebase
```bash
# Find the commit before rebase (use reflog)
git reflog show szn-build

# Reset to commit before rebase (example: szn-build@{1})
git reset --hard szn-build@{1}
```

---

## Important Notes

### ✅ DO:
- Always use `--force-with-lease` when force pushing
- Regularly rebase `szn-build` on `upstream/master` to stay current
- Create new production branches for each release
- Test production branches before deploying

### ❌ DON'T:
- Don't commit directly to production branches (`szn-prod-*`)
- Don't rebase production branches after creation
- Don't use `--force` for pushing (use `--force-with-lease`)
- Don't merge upstream into `szn-build` (always rebase)

---

## Troubleshooting

### Rebase conflicts
When rebasing, if conflicts occur:

```bash
# View conflicted files
git status

# After resolving conflicts in each file:
git add <resolved-file>

# Continue rebase
git rebase --continue

# Or abort and start over
git rebase --abort
```

### Syncing with origin after local changes
```bash
# If you've rebased locally and need to update origin
git push origin szn-build --force-with-lease

# If origin has changes you don't have locally
git fetch origin
git rebase origin/szn-build
```

### Checking if rebase is needed
```bash
# Compare your branch with upstream
git fetch upstream
git log --oneline szn-build..upstream/master

# If output is empty: you're up to date
# If output shows commits: you should rebase
```

---

## Example Timeline

**Week 1:** Start new feature
```bash
git checkout szn-build
git pull upstream master
git rebase upstream/master
# ... work on feature ...
git commit -m "feat: new feature"
git push origin szn-build --force-with-lease
```

**Week 2-4:** Continue development
```bash
git checkout szn-build
# ... more work ...
git commit -m "fix: bug fix"
git commit -m "feat: enhancement"
git push origin szn-build --force-with-lease
```

**Month-end:** Sync with upstream
```bash
git checkout szn-build
git fetch upstream
git rebase upstream/master
git push origin szn-build --force-with-lease
```

**Quarterly:** New production release
```bash
# v11.3.0 is released
git fetch upstream --tags
git checkout szn-build
git rebase upstream/master
git checkout -b szn-prod-v11.3.0 v11.3.0
git cherry-pick upstream/master..szn-build
git push origin szn-prod-v11.3.0
```

---

## Summary

1. **Development**: Always work in `szn-build`, rebase regularly on `upstream/master`
2. **Production**: Create `szn-prod-vX.Y.Z` from tags, cherry-pick all changes from `szn-build`
3. **Pushing**: Always use `--force-with-lease` when force pushing
4. **Syncing**: Fetch and rebase frequently to minimize conflicts
