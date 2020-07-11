module github.com/webdevopos/azure-k8s-autopilot

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v44.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.0
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.0 // indirect
	github.com/containrrr/shoutrrr v0.0.0-20200622190700-6520e5d4be18
	github.com/jessevdk/go-flags v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus/client_golang v0.9.3
	github.com/sirupsen/logrus v1.2.0
	golang.org/x/net v0.0.0-20200625001655-4c5254603344 // indirect
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v0.18.0
)
