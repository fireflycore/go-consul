package config

import (
	"context"

	consul "github.com/fireflycore/go-consul"
	microconfig "github.com/fireflycore/go-micro/config"
)

func NewStoreFromBootstrap(request microconfig.StoreBootstrapRequest, localLoader microconfig.LocalConfigLoader, remoteGetter microconfig.RemoteConfigGetter, payloadDecoder microconfig.PayloadDecoder, conf *Conf, opts ...microconfig.Option) (microconfig.Store, error) {
	backendConf, err := microconfig.DecodeBootstrapConfig[consul.Conf](request, localLoader, remoteGetter, payloadDecoder)
	if err != nil {
		return nil, err
	}

	client, err := consul.New(&backendConf)
	if err != nil {
		return nil, err
	}

	return NewStore(client, conf, opts...)
}

func LoadConfigFromStoreJSON[T any](ctx context.Context, store microconfig.Store, request microconfig.StoreReadRequest, payloadDecoder microconfig.PayloadDecoder) (T, error) {
	return microconfig.DecodeStoreJSON[T](ctx, store, request, payloadDecoder)
}
