.PHONY: tidy test cover lint build package clean

tidy:
	go mod tidy

# TEST_DYNAMODB_ENDPOINT=http://localhost:4566 liga o teste da idempotência contra o LocalStack.
test:
	go test ./... -race -count=1

cover:
	go test ./internal/... ./pkg/... -count=1 -covermode=atomic -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o bin/bootstrap ./cmd/notifier

package: build
	cd bin && zip -q ../notifier.zip bootstrap

clean:
	rm -rf bin notifier.zip coverage.out
