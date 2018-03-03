FROM golang:1.9
WORKDIR /go/src/github.com/blaskovicz/garmin-nest-api-proxy
COPY . .
RUN go-wrapper install ./...
EXPOSE 4091
ENV ENVIRONMENT=production PORT=4091 LOG_DEVEL=INFO
CMD ["web"]
