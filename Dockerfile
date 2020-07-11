FROM golang:1.14 as build

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
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autopilot/azure-k8s-autopilot /
USER 1000
ENTRYPOINT ["/azure-k8s-autopilot"]
