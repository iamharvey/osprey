# Do not forget to add cfdevbot
FROM golang:1.14

# Copy recipes.
ENV GO111MODULE="on"
ENV GITHUB_AUTH_TOKEN=""

WORKDIR /app
COPY main.go .
COPY go.mod .
COPY go.sum .

COPY osprey.yml /usr/local/etc/.
RUN mkdir -p /usr/local/osprey/igu

# Build service binary.
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build main.go

CMD ["/app/main"]