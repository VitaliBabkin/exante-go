# Contributing

Issues and pull requests are welcome. Before opening a pull request, run what CI runs:

```sh
gofmt -l .                       # must print nothing
go vet ./...
go test -race ./...              # -race needs cgo
golangci-lint run ./...          # v2, config in .golangci.yml
```

Tests run against `httptest` servers only; nothing may touch the live API. A behaviour change comes with a test and an
entry under "Unreleased" in `CHANGELOG.md`. The package documentation (`doc.go`) is the contract: keep it in sync with
the code.
