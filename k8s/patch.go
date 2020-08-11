package k8s

import "strings"

type (
	JsonPatch interface{}

	JsonPatchString struct {
		JsonPatch
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}

	JsonPatchObject struct {
		JsonPatch
		Op    string      `json:"op"`
		Path  string      `json:"path"`
		Value interface{} `json:"value"`
	}
)

func PatchPathEsacpe(val string) string {
	val = strings.ReplaceAll(val, "~", "~0")
	val = strings.ReplaceAll(val, "/", "~1")
	return val
}
