package agent

// DrainRequest 表示业务服务向本机 sidecar-agent 发起的摘流请求。
type DrainRequest struct {
	// AppId 表示应用标识。
	AppId string `json:"app_id"`
	// AppInstanceId 表示应用实例标识。
	AppInstanceId string `json:"app_instance_id"`
	// GracePeriod 表示摘流宽限期。
	GracePeriod string `json:"grace_period"`
}

// DeregisterRequest 表示业务服务向本机 sidecar-agent 发起的注销请求。
type DeregisterRequest struct {
	// AppId 表示应用标识。
	AppId string `json:"app_id"`
	// AppInstanceId 表示应用实例标识。
	AppInstanceId string `json:"app_instance_id"`
}
