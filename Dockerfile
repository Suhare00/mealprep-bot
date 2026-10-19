FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mealprep-bot ./cmd/bot

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /var/lib/mealprep/media \
    && chown -R app:app /var/lib/mealprep

COPY --from=build /out/mealprep-bot /usr/local/bin/mealprep-bot

USER app

ENTRYPOINT ["/usr/local/bin/mealprep-bot"]
