package webrtc

type RTCIceServerDTO struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type IceServersResponse struct {
	IceServers  []RTCIceServerDTO `json:"iceServers"`
	TurnEnabled bool              `json:"turnEnabled"`
	TTLSeconds  int               `json:"ttlSeconds"`
	ExpiresAt   string            `json:"expiresAt,omitempty"`
}
