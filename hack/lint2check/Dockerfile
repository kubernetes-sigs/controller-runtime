FROM golang

COPY lint2check /usr/bin/lint2check
COPY golangci-lint /usr/bin/golangci-lint
# make sure lint2check can find golangci-lint
ENV GO111MODULE=on

ENTRYPOINT ["/usr/bin/lint2check"]
