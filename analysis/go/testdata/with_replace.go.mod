module github.com/example/project

go 1.21

require (
	github.com/flanksource/commons v1.2.3
	github.com/local/package v0.0.0
)

replace github.com/local/package => ../local-package

replace github.com/flanksource/commons => github.com/flanksource/commons v1.3.0