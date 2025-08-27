.PHONY: help
help:
	@echo "Installing taskfile and running help..."
	@$(MAKE) install-task
	@task --list

.PHONY: install-task
install-task:
	@if ! command -v task >/dev/null 2>&1; then \
		echo "Installing Task..."; \
		go install github.com/go-task/task/v3/cmd/task@latest; \
	fi

# Mirror all common targets to task
.PHONY: build
build: install-task
	@task build

.PHONY: test
test: install-task
	@task test

.PHONY: lint
lint: install-task
	@task lint

.PHONY: fmt
fmt: install-task
	@task fmt

.PHONY: install
install: install-task
	@task install

.PHONY: clean
clean: install-task
	@task clean

.PHONY: release
release: install-task
	@task release

.PHONY: run
run: install-task
	@task run