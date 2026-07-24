.PHONY: help build test publish publish-dry-run commit claude codex

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Compile the binary locally (go build)
	go build -o cmdop-care .

test: ## Run the full test suite
	go vet ./...
	go test ./...

publish-dry-run: ## Build + verify everything, stop before any push (safe, no credentials needed)
	./release/publish.sh --dry-run

publish: ## Build, tag, and push the image + git tag — the ONE command for a real release. Reads release/.env for GHCR_TOKEN. Always run this yourself; never wired to any CI or to cmdop_go's release.
	./release/publish.sh

commit: ## Stage, AI commit, and push to main
	@g=$$(git rev-parse --git-dir) || exit 1; \
	if [ -d "$$g/rebase-merge" ] || [ -d "$$g/rebase-apply" ]; then echo "x rebase in progress - resolve, then: git rebase --continue   (abort: git rebase --abort)"; exit 1; fi; \
	if [ -f "$$g/MERGE_HEAD" ]; then echo "x merge in progress - resolve, then: git merge --continue   (abort: git merge --abort)"; exit 1; fi; \
	if [ -f "$$g/CHERRY_PICK_HEAD" ]; then echo "x cherry-pick in progress - resolve, then: git cherry-pick --continue   (abort: git cherry-pick --abort)"; exit 1; fi; \
	if [ -f "$$g/REVERT_HEAD" ]; then echo "x revert in progress - resolve, then: git revert --continue   (abort: git revert --abort)"; exit 1; fi; \
	if [ -n "$$(git ls-files --unmerged)" ]; then echo "x unresolved conflicts:"; git diff --name-only --diff-filter=U | sed 's/^/    /'; echo "  resolve them, then: git add <file>"; exit 1; fi; \
	branch=$$(git symbolic-ref --quiet --short HEAD) || { echo "x detached HEAD at $$(git rev-parse --short HEAD) - commits made here get orphaned"; echo "  keep this work: git switch -c <branch>   |   discard it: git switch main"; exit 1; }; \
	[ "$$branch" = "main" ] || { echo "x on branch '$$branch', not main - push it explicitly yourself"; exit 1; }; \
	git add . ; \
	if ! git diff --cached --quiet; then orc commit -y || exit 1; fi ; \
	git fetch -q origin main ; \
	git merge-base --is-ancestor origin/main HEAD || git rebase origin/main || { echo "x rebase conflicts - resolve them, then: git push origin main"; exit 1; } ; \
	git push origin main

claude: ## Start Claude Code with permissions bypassed
	@claude --dangerously-skip-permissions --chrome

codex: ## Start Codex with approvals and sandbox bypassed
	@codex --dangerously-bypass-approvals-and-sandbox
