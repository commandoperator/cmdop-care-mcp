# cmdop-care — minimal, non-root, read-only-friendly OCI image.
#
# Build stage: static Go binary, no CGO (matches the private cmdop_go house
# rule of cross-OS, dependency-light builds — this module has zero cgo
# dependency, confirmed by `go build ./...` with CGO_ENABLED=0 below).
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# TARGETOS/TARGETARCH are supplied by buildx per requested --platform, so a
# single `docker buildx build --platform linux/amd64,linux/arm64 ...` produces
# a correct native binary for each manifest in the resulting image index —
# never hardcode GOARCH here (a prior amd64-only build shipped a mismatched
# binary inside an arm64-labeled manifest on an Apple Silicon build host).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/cmdop-care .

# Runtime stage: distroless static base — no shell, no package manager, no
# unnecessary packages.
#
# Pinned by digest (resolved 2026-07-18 from the `nonroot` tag). Do NOT
# publish this image with a floating tag — the security review (§11, §13)
# requires a pinned digest so a supply-chain change to the base image cannot
# silently change what ships. Re-resolve deliberately, on its own reviewed
# change, with:
#
#   docker pull gcr.io/distroless/static-debian12:nonroot
#   docker inspect --format='{{index .RepoDigests 0}}' gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b

# distroless "nonroot" variants already run as uid 65532 (user "nonroot");
# stated explicitly so this is never accidentally left root if the base
# variant is ever swapped for one that defaults to root.
USER 65532:65532

COPY --from=build /out/cmdop-care /cmdop-care

# OCI label MUST exactly match server.json's "name" field (security review
# §13, publication checklist #9).
LABEL io.modelcontextprotocol.server.name="io.github.commandoperator/cmdop-care"

# stdio transport — no listening port, nothing to EXPOSE.
ENTRYPOINT ["/cmdop-care"]
