// Package trafficpolicy contains the immutable terms attached to a purchase.
package trafficpolicy

type Rule struct {
	Devices   int `json:"devices"`
	TrafficGB int `json:"trafficGb"`
}

type Terms struct {
	Enabled  bool   `json:"enabled"`
	Enforced bool   `json:"enforced"`
	Rules    []Rule `json:"rules"`
}
