#############################################
# Build
#############################################
FROM --platform=$BUILDPLATFORM golang:1.19-alpine as build

RUN apk upgrade --no-cache --force
RUN apk add --update build-base make git curl

WORKDIR /go/src/github.com/webdevops/azure-k8s-autopilot

# Dependencies
COPY go.mod go.sum .
RUN go mod download

COPY . .
RUN make test
ARG TARGETOS TARGETARCH

# kubectl
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/${TARGETOS}/${TARGETARCH}/kubectl
RUN chmod +x kubectl

# Compile
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make build

#############################################
# Test
#############################################
FROM gcr.io/distroless/static as test
USER 0:0
WORKDIR /app
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/azure-k8s-autopilot .
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/kubectl .
RUN ["./azure-k8s-autopilot", "--help"]
RUN ["./kubectl", "version", "--client=true"]

#############################################
# Final
#############################################
FROM gcr.io/distroless/static
ENV LOG_JSON=1
WORKDIR /
COPY --from=test /app .
USER 1000:1000
ENTRYPOINT ["/azure-k8s-autopilot"]
