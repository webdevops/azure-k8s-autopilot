FROM golang:1.14 as build

WORKDIR /go/src/github.com/webdevops/azure-k8s-autorepair

# Get deps (cached)
COPY ./go.mod /go/src/github.com/webdevops/azure-k8s-autorepair
COPY ./go.sum /go/src/github.com/webdevops/azure-k8s-autorepair
RUN go mod download

# Compile
COPY ./ /go/src/github.com/webdevops/azure-k8s-autorepair
RUN make lint
RUN make build
RUN ./azure-k8s-autorepair --help

#############################################
# FINAL IMAGE
#############################################
FROM gcr.io/distroless/static
COPY --from=build /go/src/github.com/webdevops/azure-k8s-autorepair/azure-k8s-autorepair /
USER 1000
ENTRYPOINT ["/azure-k8s-autorepair"]
