# This dockerfile is used to build Mattermost rtcd
# A multi stage build, with golang used as a builder
# and gcr.io/distroless/static as runner
ARG GO_VERSION
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder
ARG TARGETOS
ARG TARGETARCH
ARG GO_BUILD_PLATFORMS=${TARGETOS}-${TARGETARCH}

# Setup directories structure and compile
COPY . /src
WORKDIR /src
RUN make go-build

# Shrink final image since we only need the rtcd binary
# and use distroless container image as runner for security
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static@sha256:d6fa9db9548b5772860fecddb11d84f9ebd7e0321c0cb3c02870402680cc315f AS runner
ARG TARGETOS
ARG TARGETARCH
COPY --from=builder /src/dist/rtcd-${TARGETOS}-${TARGETARCH} /opt/rtcd/bin/rtcd

# Create and use unprivileged user to run the service
COPY ./build/group ./build/passwd /etc/

USER calls

WORKDIR /opt/rtcd/bin
ENTRYPOINT ["./rtcd"]
