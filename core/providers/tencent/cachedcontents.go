package tencent

import (
	providerUtils "github.com/pin-gou/pg-gateway/core/providers/utils"
	"github.com/pin-gou/pg-gateway/core/schemas"
)

func (provider *TencentProvider) CachedContentCreate(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostCachedContentCreateRequest) (*schemas.BifrostCachedContentCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

func (provider *TencentProvider) CachedContentList(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostCachedContentListRequest) (*schemas.BifrostCachedContentListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

func (provider *TencentProvider) CachedContentRetrieve(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostCachedContentRetrieveRequest) (*schemas.BifrostCachedContentRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

func (provider *TencentProvider) CachedContentUpdate(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostCachedContentUpdateRequest) (*schemas.BifrostCachedContentUpdateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

func (provider *TencentProvider) CachedContentDelete(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostCachedContentDeleteRequest) (*schemas.BifrostCachedContentDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}