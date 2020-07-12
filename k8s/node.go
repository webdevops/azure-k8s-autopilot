package k8s

import (
	v1 "k8s.io/api/core/v1"
)

type (
	Node struct {
		*v1.Node
	}
)
