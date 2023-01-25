# This dockerfile is used to build Mattermost rtcd
# A multi stage build, with golang used as a builder
# and gcr.io/distroless/static as runner
ARG GO_IMAGE=golang:1.18@sha256:fa71e1447cb0241324162a6c51297206928d755b16142eceec7b809af55061e5
# hadolint ignore=DL3006
FROM ${GO_IMAGE} as builder

#GO_BUILD_PLATFORMS holds the platforms that we will build the docker image against
ARG GO_BUILD_PLATFORMS=linux-amd64

# Setup directories structure and compile
COPY . /src
WORKDIR /src
RUN make go-build

# Shrink final image since we only need the rtcd binary
# and use distroless container image as runner for security
FROM gcr.io/distroless/static@sha256:d6fa9db9548b5772860fecddb11d84f9ebd7e0321c0cb3c02870402680cc315f as runner
COPY --from=builder /src/dist/rtcd-linux-amd64 /opt/rtcd/bin/rtcd

# Create and use unprivileged user to run the service
COPY ./build/group ./build/passwd /etc/

USER calls

WORKDIR /opt/rtcd/bin
ENTRYPOINT ["./rtcd"]
