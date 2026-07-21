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
	@git add . && orc commit -y && git push origin main

claude: ## Start Claude Code with permissions bypassed
	@claude --dangerously-skip-permissions --chrome

codex: ## Start Codex with approvals and sandbox bypassed
	@codex --dangerously-bypass-approvals-and-sandbox
