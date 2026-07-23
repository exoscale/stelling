VERSION --try 0.8

ARG --global GO_VERSION=1.26.5

source:
    FROM library/golang:${GO_VERSION}-alpine
    WORKDIR /build
    COPY go.mod go.sum ./
    COPY . .

format:
    FROM +source
    RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
    RUN golangci-lint fmt ./...
    RUN golangci-lint run --fix ./...
    RUN rm -rf ./.tmp-* # Remove temporary dirs
    SAVE ARTIFACT ./* AS LOCAL .

vulncheck:
    FROM +source
    RUN go install golang.org/x/vuln/cmd/govulncheck@latest
    RUN govulncheck ./...
