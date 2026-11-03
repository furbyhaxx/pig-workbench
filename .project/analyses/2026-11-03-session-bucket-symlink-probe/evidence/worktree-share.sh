#!/bin/bash
# Share the main worktree's .project/ and .env into a linked worktree without git noise.
set -e; T=/tmp/wb-wtprobe; rm -rf $T; mkdir -p $T/repo; cd $T/repo; git init -q -b main
mkdir -p .project/specs src; echo spec1 > .project/specs/a.md; echo code > src/x; echo 'SECRET=1' > .env; echo .env > .gitignore
git add .; git -c user.email=a@b -c user.name=t commit -qm init
git worktree add -q .worktrees/feat2 -b feat2; cd .worktrees/feat2
git config extensions.worktreeConfig true
git config --worktree sparse.expectFilesOutsideOfPatterns true   # must precede sparse-checkout set
git sparse-checkout set --no-cone '/*' '!/.project/'
ln -s ../../.project .project; ln -s ../../.env .env
echo '/.project' >> "$(git rev-parse --git-common-dir)/info/exclude"
git status --short            # expect: empty
git ls-files -v | grep project  # expect: S .project/specs/a.md
echo x >> src/x; git -c user.email=a@b -c user.name=t commit -qam w; git show --stat --oneline HEAD | tail -1
