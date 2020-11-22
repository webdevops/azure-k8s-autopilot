FROM golang:1.15 as build
ARG TARGETOS
ARG TARGETARCH
# kubectl
WORKDIR /
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/$TARGETOS/$TARGETARCH/kubectl
RUN chmod +x /kubectl
RUN /kubectl version --client=true

WORKDIR /go/src/github.com/webdevops/azure-k8s-autopilot

# Get deps (cached)
COPY ./go.mod /go/src/github.com/webdevops/azure-k8s-autopilot
COPY ./go.sum /go/src/github.com/webdevops/azure-k8s-autopilot
COPY ./Makefile /go/src/github.com/webdevops/azure-k8s-autopilot
RUN make dependencies

# Compile
COPY ./ /go/src/github.com/webdevops/azure-k8s-autopilot
RUN make test
RUN make lint
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
