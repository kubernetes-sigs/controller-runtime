FROM golang:1.13 as build

RUN mkdir /build
WORKDIR /build
COPY checkerator.go go.mod go.sum /build/
RUN CGO_ENABLED=0 go build -o checkerator .

FROM gcr.io/distroless/static
COPY --from=build /build/checkerator /
ENTRYPOINT ["/checkerator"]
