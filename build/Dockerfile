# This dockerfile is used to build Mattermost rtcd
# A multi stage build, with golang:1.17.9-alpine used as a builder
# and gcr.io/distroless/static as runner
FROM golang:1.17.9-alpine@sha256:5c2fcfeb58ad9d4948d94e7b2d0432a9bc38bee0f8dfb41d383f38a18b75c38d as builder

# Setup directories structure and compile
COPY . /src
WORKDIR /src
RUN apk add \
    make=4.3-r0 \
    git=2.34.2-r0 \
    --no-cache \
    && make build

# Shrink final image since we only need the rtcd binary
# and use distroless container image as runner for security
FROM gcr.io/distroless/static@sha256:d6fa9db9548b5772860fecddb11d84f9ebd7e0321c0cb3c02870402680cc315f as runner
COPY --from=builder /src/dist/rtcd-linux-amd64 /opt/rtcd/bin/rtcd

# We should refrain from running as privileged user
# Run as UID for nobody
USER 65534

WORKDIR /opt/rtcd/bin
ENTRYPOINT ["./rtcd"]
