# GitOwl

GitOwl is a lightweight macOS Git client for reviewing repository changes and
history, managing branches and worktrees, and writing commit messages. This
file defines the domain terms used in the code, the interface and the README.
Architecture decisions are in [docs/adr/](docs/adr/).

## Repositories

**Repository**: A local Git project whose history and branches are shared by its primary worktree and any linked worktrees.

**Recent repository**: A repository remembered locally after it was successfully opened in GitOwl. The list keeps the last 10.

**Pinned repository**: A recent repository kept at the top of the repository list across launches. Pinning does not open it.

**Welcome screen**: The view shown when no repository is open, with entry points to open a repository or choose one from the recent list.

**Close repository**: Ending the current repository view and returning to the welcome screen while keeping GitOwl open. The repository, its working tree changes and its staging area remain intact.
_Avoid_: Delete repository, remove worktree

**Worktree**: A checked-out working directory of a repository, with its own working tree changes, staging area and HEAD. The primary worktree and linked worktrees share repository history and branches.

**New worktree**: A linked worktree created in a new folder beside the main worktree, for an existing local branch or a new branch from the current HEAD.

**Remove worktree**: Deleting a clean linked worktree's folder and Git's record of it. Its branch stays.

**Prune worktrees**: Removing Git's records of worktrees whose folder no longer exists, without deleting any folder.
_Avoid_: Clean worktrees

## Branches and remotes

**Branch**: A named line of development in a repository.

**Upstream branch**: The branch a local branch tracks for comparing and exchanging commits with a remote.

**Browse branch**: Reading a branch's history without changing the checked-out branch or worktree.
_Avoid_: Checkout

**Switch branch**: Checking out a local branch, so the worktree and HEAD move to it. Done by double-clicking the branch or from its menu.

**Detached HEAD**: A worktree checked out at a particular commit rather than a named local branch.

**Checkout commit**: Checking out a selected historical commit with a detached HEAD.
_Avoid_: Browse commit

**Fetch**: Updating remote-tracking branches from remote history. It never changes files, the index or local branches. Fetch is manual.

**Fast-forward pull**: Fetching upstream changes and updating the current branch only when it can advance along the existing history without creating a merge commit or rewriting commits.

**Pull strategy**: How Pull brings in the upstream: **Merge** (the default, recording a merge commit when the branches have diverged), **Rebase** (replaying the current branch onto the fetched commits), or **Fast-forward only**.

**Autostash**: Stashing local changes before a pull. It is on by default in the Pull popover.

**Push**: Sending the current branch to its upstream as a fast-forward. Push does not send tags and sends one branch at a time.

**Force with lease**: A push that overwrites the upstream branch only when it still matches the commit last fetched. It is an option in the Push popover.

**Publish branch**: The first push of a local branch that has no upstream. It creates the remote branch and sets it as the upstream.

**Unpushed commit**: A commit reachable from the selected local branch but not from its upstream, according to the remote history last fetched.

**Incoming commit**: A commit reachable from the selected branch's upstream but not from the selected branch, according to the remote history last fetched.

**Branch deletion**: Deleting a local branch with Git's safe delete. GitOwl does not force-delete branches.

## Changes and staging

**Working tree changes**: File changes in the open repository that have not yet been included in a commit. Shown as **Current changes** in the interface.

**Diff**: A comparison of file content between two Git states, presented for review.

**Change hunk**: A group of changed lines and surrounding context presented together in a diff. It can be staged or unstaged as a unit.

**Staged changes**: Working tree changes selected for inclusion in a future commit.

**Unstaged changes**: Changes to tracked files that are not in the staging area. A file may have both staged and unstaged changes; the two parts belong to separate diffs.

**Untracked files**: Files in the working copy that Git is not tracking and that have not been staged for inclusion in a commit.

**Stage all**: Selecting all eligible changes in the current worktree for the next commit, including tracked modifications, deletions and untracked files that are not ignored.

**Unstage all**: Removing all staged changes from the next commit while preserving the current worktree's file contents.
_Avoid_: Cancel all, discard all

**Discard unstaged changes**: Restoring a tracked file's working tree content to its staged version while preserving the staging area. It runs one file at a time, after confirmation.
_Avoid_: Unstage, cancel all

**Conflict**: A file with merge markers from a merge, rebase, cherry-pick or revert. Conflicts are shown on Current changes. A revert's conflicts can be continued or aborted in GitOwl; the others are resolved in an editor or in Terminal.

## Stash

**Stash**: A saved copy of working tree changes, kept apart from the branch history. A stash can be applied, popped, dropped, renamed or turned into a new branch.

**Stash all changes**: A stash that includes untracked files along with tracked changes.

**Apply stash**: Restoring a stash to the working tree and keeping the stash entry.

**Pop stash**: Restoring a stash to the working tree and removing the stash entry.

**Drop stash**: Removing a stash entry without restoring it, after confirmation.

## Commit history

**Commit history**: The repository's recorded commits, browsed for context and review. Shown as **All commits** in the interface.

**History graph**: A view of commits and their parent relationships within the selected history scope.

**History scope**: The branch being browsed and, when it has one, its upstream branch.

**Commit search**: Finding commits in the history of a branch by text in messages, authors and emails, by a commit hash, or by a file or folder path.

**Revert commit**: A new commit on the current branch that reverses the changes introduced by a selected earlier non-merge commit.
_Avoid_: Discard unstaged changes, reset

**Cherry-pick**: Applying one commit's changes onto the current branch as a new commit.

**Merge**: Combining another branch into the current branch. It records a merge commit.

**Rebase**: Replaying the current branch onto another branch, rewriting its commits.

**Mixed reset**: Moving the current branch and the staging area to a commit while keeping the working tree files as they are.

Merge, rebase and cherry-pick need a clean working tree and no operation in progress.

## Commit creation

**Commit summary**: The first line of a commit message, describing its main change.

**Commit description**: The optional body of a commit message, separated from its summary by a blank line.

**Commit and push**: Creating a local commit and then pushing the current branch. A failed push leaves the local commit in place.

**Amend previous commit**: Replacing the current branch's latest commit with a revised message, staged content, or both. It is available only while the upstream, as last fetched, does not contain that commit.

## AI commit drafts

**Commit message draft**: Editable text suggested by AI from staged changes. A draft becomes part of the repository's history only when the user explicitly creates a commit.

**Draft suggestion**: An AI-generated commit message offered for acceptance before it becomes the editable message.

**Commit message prompt**: Instructions that define the style or format expected from generated commit message drafts.

**Model context**: The context size, in tokens, of the model that writes drafts, set in AI Settings (8K by default). GitOwl plans each draft request to fit it, keeping room for the reply.

**Token estimate**: GitOwl's own, model-independent count of what a request needs of the model context. It is deliberately high and is not the model's tokenizer.

**Shortened request**: A draft request that does not show every staged change in full: some files as statistics only, or less of their diff. The request says which and why.
_Avoid_: Summary, compressed diff

**View request**: Seeing the exact request Generate Draft would send, without sending it.
_Avoid_: Preview
