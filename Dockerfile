FROM golang:1.14 as build

# kubectl
WORKDIR /
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
RUN chmod +x /kubectl
RUN /kubectl version --client=true --short=true

WORKDIR /go/src/github.com/webdevops/azure-k8s-autopilot

# Get deps (cached)
COPY ./go.mod /go/src/github.com/webdevops/azure-k8s-autopilot
COPY ./go.sum /go/src/github.com/webdevops/azure-k8s-autopilot
RUN go mod download

# Compile
COPY ./ /go/src/github.com/webdevops/azure-k8s-autopilot
RUN make lint
RUN make build
RUN ./azure-k8s-autopilot --help

#############################################
# FINAL IMAGE
#############################################
FROM gcr.io/distroless/static

ENV LOG_JSON=1\
    DRAIN_KUBECTL=/kubectl

COPY --from=build /kubectl /
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/azure-k8s-autopilot /
USER 1000
ENTRYPOINT ["/azure-k8s-autopilot"]
