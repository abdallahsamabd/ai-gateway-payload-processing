package dynamicmetadata

import (
	"encoding/json"

	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
)

const pseudoHeader = "x-ipp-internal-dynamic-metadata"

type entry struct {
	Namespace string   `json:"ns"`
	Key       string   `json:"key"`
	Values    []string `json:"values"`
}

// SetEndpointSubset sets the x-gateway-destination-endpoint-subset dynamic
// metadata on the ext-proc response via a pseudo-header that the stream
// wrapper strips and converts to DynamicMetadata before Envoy sees it.
func SetEndpointSubset(request *requesthandling.InferenceRequest, endpoints []string) {
	e := entry{
		Namespace: "envoy.lb.subset_hint",
		Key:       "x-gateway-destination-endpoint-subset",
		Values:    endpoints,
	}
	data, _ := json.Marshal(e)
	request.SetHeader(pseudoHeader, string(data))
}
