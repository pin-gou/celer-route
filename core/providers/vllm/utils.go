package vllm

import (
	"github.com/bytedance/sonic"
	providerUtils "github.com/pin-gou/celer-route/core/providers/utils"
	schemas "github.com/pin-gou/celer-route/core/schemas"
)

func HandleVLLMResponse[T any](responseBody []byte, response *T, requestBody []byte, sendBackRawRequest bool, sendBackRawResponse bool) (rawRequest interface{}, rawResponse interface{}, bifrostErr *schemas.BifrostError) {
	var errorResp schemas.BifrostError
	rawRequest, rawResponse, bifrostErr = providerUtils.HandleProviderResponse(responseBody, response, requestBody, sendBackRawRequest, sendBackRawResponse)
	if bifrostErr != nil {
		return rawRequest, rawResponse, bifrostErr
	}
	if err := sonic.Unmarshal(responseBody, &errorResp); err == nil && errorResp.Error != nil && errorResp.Error.Message != "" {
		return rawRequest, rawResponse, &errorResp
	}
	return rawRequest, rawResponse, nil
}
