.PHONY: gopas test

gopas:
	go build -o gopas ./cmd/gopas

test:
	go test ./... -count=1
