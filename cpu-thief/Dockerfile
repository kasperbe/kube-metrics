FROM golang:alpine AS builder

WORKDIR $GOPATH/src/kasperbe/cpu-thief
COPY . .
RUN go build -o /go/bin/thief

FROM scratch

EXPOSE 8080

COPY --from=builder /go/bin/thief /go/bin/thief

ENTRYPOINT ["/go/bin/thief"]

