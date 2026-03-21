package consul

type Conf struct {
	Address    string `json:"address"`
	Scheme     string `json:"scheme"`
	Datacenter string `json:"datacenter"`
	Token      string `json:"token"`

	TLS *TLS `json:"tls"`
}

type TLS struct {
	Enable             bool   `json:"enable"`
	CaFile             string `json:"ca_file"`
	CertFile           string `json:"cert_file"`
	KeyFile            string `json:"key_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}
