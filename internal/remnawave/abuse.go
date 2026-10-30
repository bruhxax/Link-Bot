package remnawave

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ObservedIP struct {
	IP       string    `json:"ip"`
	Node     string    `json:"node"`
	LastSeen time.Time `json:"lastSeen,omitempty"`
}

func (r *Client) GetUserHWIDDevices(ctx context.Context, id int64, userUUID uuid.UUID) ([]UserDevice, error) {
	return r.getUserHWIDDevices(ctx, id, userUUID)
}

// FetchUserIPs reads the panel's asynchronous connection snapshot. A failed or
// incomplete job is an error, never an empty observation.
func (r *Client) FetchUserIPs(ctx context.Context, id int64, userUUID uuid.UUID) ([]ObservedIP, error) {
	if id <= 0 && userUUID == uuid.Nil {
		return nil, errors.New("missing panel user identity")
	}
	modern := id > 0
	start := "/api/connections/by-user/" + strconv.FormatInt(id, 10)
	result := "/api/connections/by-user/"
	if !modern {
		start = "/api/ip-control/fetch-ips/" + userUUID.String()
		result = "/api/ip-control/fetch-ips/result/"
	}
	var job struct {
		Response struct {
			JobID string `json:"jobId"`
		} `json:"response"`
	}
	err := r.doAPIJSON(ctx, http.MethodPost, start, nil, &job)
	if err != nil && modern && userUUID != uuid.Nil && isLegacyFallbackError(err) {
		start = "/api/ip-control/fetch-ips/" + userUUID.String()
		result = "/api/ip-control/fetch-ips/result/"
		err = r.doAPIJSON(ctx, http.MethodPost, start, nil, &job)
	}
	if err != nil {
		return nil, err
	}
	if job.Response.JobID == "" {
		return nil, errors.New("remnawave returned no IP job ID")
	}
	for attempt := 0; attempt < 24; attempt++ {
		var payload struct {
			Response struct {
				IsCompleted bool `json:"isCompleted"`
				IsFailed    bool `json:"isFailed"`
				Result      *struct {
					Success bool `json:"success"`
					Nodes   []struct {
						NodeName string `json:"nodeName"`
						IPs      []struct {
							IP       string    `json:"ip"`
							LastSeen time.Time `json:"lastSeen"`
						} `json:"ips"`
					} `json:"nodes"`
				} `json:"result"`
			} `json:"response"`
		}
		if err := r.doAPIJSON(ctx, http.MethodGet, result+url.PathEscape(job.Response.JobID), nil, &payload); err != nil {
			return nil, err
		}
		if payload.Response.IsFailed {
			return nil, errors.New("remnawave IP job failed")
		}
		if payload.Response.IsCompleted {
			if payload.Response.Result == nil || !payload.Response.Result.Success {
				return nil, errors.New("incomplete remnawave IP result")
			}
			seen := map[string]ObservedIP{}
			for _, node := range payload.Response.Result.Nodes {
				for _, item := range node.IPs {
					address, err := netip.ParseAddr(strings.TrimSpace(item.IP))
					if err != nil {
						continue
					}
					ip := address.Unmap().String()
					old, exists := seen[ip]
					if !exists || item.LastSeen.After(old.LastSeen) {
						seen[ip] = ObservedIP{IP: ip, Node: node.NodeName, LastSeen: item.LastSeen}
					}
				}
			}
			ips := make([]ObservedIP, 0, len(seen))
			for _, item := range seen {
				ips = append(ips, item)
			}
			sort.Slice(ips, func(i, j int) bool { return ips[i].IP < ips[j].IP })
			return ips, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("remnawave IP job %s timed out", job.Response.JobID)
}
