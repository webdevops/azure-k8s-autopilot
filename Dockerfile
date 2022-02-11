FROM golang:1.17-alpine as build

ARG TARGETOS
ARG TARGETARCH

RUN apk upgrade --no-cache --force
RUN apk add --update build-base make git curl

# kubectl
WORKDIR /
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/$TARGETOS/$TARGETARCH/kubectl
RUN chmod +x /kubectl
RUN /kubectl version --client=true

WORKDIR /go/src/github.com/webdevops/azure-k8s-autopilot

# Compile
COPY ./ /go/src/github.com/webdevops/azure-k8s-autopilot
RUN make test
RUN make dependencies
RUN make build
RUN ./azure-k8s-autopilot --help

#############################################
# FINAL IMAGE
#############################################
FROM gcr.io/distroless/static

ENV LOG_JSON=1\
    DRAIN_KUBECTL=/kubectl \
    LEASE_ENABLE=1

COPY --from=build /kubectl /
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/azure-k8s-autopilot /
USER 1000
ENTRYPOINT ["/azure-k8s-autopilot"]
