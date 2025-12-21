#############################################
# Build
#############################################
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

RUN apk upgrade --no-cache --force
RUN apk add --update build-base make git curl

WORKDIR /go/src/github.com/webdevops/azure-k8s-autopilot

# Dependencies
COPY go.mod go.sum .
RUN go mod download

COPY . .
RUN make test
RUN make build # warmup
ARG TARGETOS TARGETARCH

# kubectl
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/${TARGETOS}/${TARGETARCH}/kubectl
RUN chmod +x kubectl

# Compile
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make build

#############################################
# Test
#############################################
FROM gcr.io/distroless/static AS test
USER 0:0
WORKDIR /app
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/azure-k8s-autopilot .
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/kubectl .
RUN ["./azure-k8s-autopilot", "--help"]
RUN ["./kubectl", "version", "--client=true", "--output=yaml"]

#############################################
# final-azcli
#############################################
FROM mcr.microsoft.com/azure-cli AS final-azcli
ENV LOG_JSON=1
WORKDIR /
COPY --from=test /app .
USER 1000:1000
ENTRYPOINT ["/azure-k8s-autopilot"]

#############################################
# Final
#############################################
FROM gcr.io/distroless/static AS final-static
ENV LOG_JSON=1 \
    LEASE_ENABLE=true
WORKDIR /
COPY --from=test /app .
USER 1000:1000
ENTRYPOINT ["/azure-k8s-autopilot"]
