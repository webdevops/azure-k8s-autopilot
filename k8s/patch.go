package k8s

import "strings"

type (
	PatchStringValue struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	PatchRemove struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
	}
)

func PatchPathEsacpe(val string) (string) {
	val = strings.ReplaceAll(val, "~", "~0")
	val = strings.ReplaceAll(val, "/", "~1")
	return val
}
