goimports -w .
golangci-lint --version
golangci-lint run ./... -v -c .github/linters/.golangci.yaml --fix
