FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /function .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /function /function
USER 65532:65532
ENTRYPOINT ["/function"]
