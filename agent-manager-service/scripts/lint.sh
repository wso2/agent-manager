goimports -w .
golangci-lint --version
# Run all linters with --fix except goheader (to prevent modifying existing copyright years)
golangci-lint run ./... -v -c .github/linters/.golangci.yaml --fix --disable goheader
# Run goheader separately without --fix to only verify headers exist without modifying them
golangci-lint run ./... -v -c .github/linters/.golangci.yaml --enable-only goheader
