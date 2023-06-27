package main

import (
	"encoding/json"
	"testing"

	"gotest.tools/assert"
)

var (
	requestBody = `
{
  "images": {
    "containers": {
      "tomcat": {
        "registry": "https://ghcr.io",
        "path": "tomcat",
        "name": "tomcat",
        "tag": "9",
        "jsonPointer": "spec/container/0/image"
      }
    },
    "initContainers": {
      "vault": {
        "registry": "https://ghcr.io",
        "path": "vault",
        "name": "vault",
        "tag": "v3",
        "jsonPointer": "spec/initContainer/0/image"
      }
    }
  }
}`
)

func TestInput(t *testing.T) {
	var requestData RequestData
	err := json.Unmarshal([]byte(requestBody), &requestData)
	assert.NilError(t, err)
	assert.Equal(t, requestData.Images.Containers["tomcat"].Name, "tomcat")
	assert.Equal(t, requestData.Images.Containers["tomcat"].Pointer, "spec/container/0/image")
	assert.Equal(t, requestData.Images.InitContainers["vault"].Name, "vault")
	assert.Equal(t, requestData.Images.InitContainers["vault"].Pointer, "spec/initContainer/0/image")
}
