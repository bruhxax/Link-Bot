package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

// PanelUser is the stable user model used by Link-Bot. Remnawave 3.x uses the
// numeric ID, while UUID is retained only for compatibility with 2.x panels.
type PanelUser struct {
	ID                   int64            `json:"id"`
	UUID                 uuid.UUID        `json:"uuid,omitempty"`
	ShortUUID            string           `json:"shortUuid,omitempty"`
	Username             string           `json:"username"`
	Status               string           `json:"status"`
	ExpireAt             time.Time        `json:"expireAt"`
	TelegramID           *int64           `json:"telegramId"`
	Description          *string          `json:"description"`
	SubscriptionURL      string           `json:"subscriptionUrl"`
	TrafficLimitBytes    int64            `json:"trafficLimitBytes"`
	TrafficLimitStrategy string           `json:"trafficLimitStrategy"`
	HwidDeviceLimit      *int             `json:"hwidDeviceLimit"`
	ExternalSquadUUID    *uuid.UUID       `json:"externalSquadUuid"`
	Tag                  *string          `json:"tag"`
	ActiveInternalSquads []PanelSquad     `json:"activeInternalSquads"`
	UserTraffic          PanelUserTraffic `json:"userTraffic"`
}

type PanelSquad struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

type PanelUserTraffic struct {
	UsedTrafficBytes         int64      `json:"usedTrafficBytes"`
	LifetimeUsedTrafficBytes int64      `json:"lifetimeUsedTrafficBytes"`
	OnlineAt                 *time.Time `json:"onlineAt"`
}

func (u *PanelUser) GetSubscriptionUrl() string {
	if u == nil {
		return ""
	}
	return u.SubscriptionURL
}

func (u *PanelUser) GetExpireAt() time.Time {
	if u == nil {
		return time.Time{}
	}
	return u.ExpireAt
}

type remnawaveAPIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *remnawaveAPIError) Error() string {
	if e == nil {
		return "remnawave request failed"
	}
	if e.Body == "" {
		return fmt.Sprintf("remnawave request failed: %s", e.Status)
	}
	return fmt.Sprintf("remnawave request failed: %s: %s", e.Status, e.Body)
}

func (r *Client) doAPIJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &remnawaveAPIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(data)),
		}
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode remnawave response: %w", err)
	}
	return nil
}

func isLegacyFallbackError(err error) bool {
	var apiErr *remnawaveAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func (r *Client) streamUsers(ctx context.Context, filters url.Values) ([]PanelUser, error) {
	if filters == nil {
		filters = url.Values{}
	}
	filters = cloneValues(filters)
	if filters.Get("size") == "" {
		filters.Set("size", "500")
	}

	users := make([]PanelUser, 0, 500)
	cursor := ""
	for page := 0; page < 10000; page++ {
		if cursor == "" {
			filters.Del("cursor")
		} else {
			filters.Set("cursor", cursor)
		}

		var payload struct {
			Response struct {
				Users      []PanelUser     `json:"users"`
				NextCursor json.RawMessage `json:"nextCursor"`
				HasMore    bool            `json:"hasMore"`
			} `json:"response"`
		}
		if err := r.doAPIJSON(ctx, http.MethodGet, "/api/users/stream?"+filters.Encode(), nil, &payload); err != nil {
			return nil, err
		}
		users = append(users, payload.Response.Users...)
		if !payload.Response.HasMore {
			return users, nil
		}
		next := cursorString(payload.Response.NextCursor)
		if next == "" || next == cursor {
			return nil, errors.New("remnawave users stream returned an invalid cursor")
		}
		cursor = next
	}
	return nil, errors.New("remnawave users stream exceeded pagination limit")
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func cursorString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return strings.TrimSpace(value)
		}
	}
	return trimmed
}

func (r *Client) legacyUsers(ctx context.Context) ([]PanelUser, error) {
	pager := remapi.NewPaginationHelper(250)
	users := make([]PanelUser, 0, 250)
	for {
		resp, err := r.client.Users().GetAllUsers(ctx, pager.Limit, pager.Offset)
		if err != nil {
			return nil, err
		}
		response, ok := resp.(*remapi.GetAllUsersResponse)
		if !ok {
			return nil, errors.New("unknown legacy users response type")
		}
		items := response.GetResponse().Users
		for i := range items {
			users = append(users, panelUserFromLegacy(&items[i]))
		}
		if len(items) < pager.Limit || !pager.NextPage() {
			return users, nil
		}
	}
}

func panelUserFromLegacy(user *remapi.UserItemInfo) PanelUser {
	if user == nil {
		return PanelUser{}
	}
	result := PanelUser{
		ID:                   int64(user.ID),
		UUID:                 user.UUID,
		Username:             strings.TrimSpace(user.Username),
		ExpireAt:             user.ExpireAt.UTC(),
		SubscriptionURL:      strings.TrimSpace(user.SubscriptionUrl),
		ActiveInternalSquads: make([]PanelSquad, 0, len(user.ActiveInternalSquads)),
	}
	if value, ok := user.Status.Get(); ok {
		result.Status = string(value)
	}
	if value, ok := user.TelegramId.Get(); ok && value > 0 {
		converted := int64(value)
		result.TelegramID = &converted
	}
	if value, ok := user.Description.Get(); ok {
		copied := value
		result.Description = &copied
	}
	for _, squad := range user.ActiveInternalSquads {
		result.ActiveInternalSquads = append(result.ActiveInternalSquads, PanelSquad{UUID: squad.UUID, Name: strings.TrimSpace(squad.Name)})
	}

	// The generated v2 type exposes several fields through optional wrappers.
	// Re-marshalling keeps the adapter resilient across minor SDK versions.
	if encoded, err := json.Marshal(user); err == nil {
		var stats struct {
			ShortUUID            string           `json:"shortUuid"`
			TrafficLimitBytes    int64            `json:"trafficLimitBytes"`
			TrafficLimitStrategy string           `json:"trafficLimitStrategy"`
			HwidDeviceLimit      *int             `json:"hwidDeviceLimit"`
			ExternalSquadUUID    *uuid.UUID       `json:"externalSquadUuid"`
			Tag                  *string          `json:"tag"`
			UserTraffic          PanelUserTraffic `json:"userTraffic"`
		}
		if json.Unmarshal(encoded, &stats) == nil {
			result.ShortUUID = stats.ShortUUID
			result.TrafficLimitBytes = stats.TrafficLimitBytes
			result.TrafficLimitStrategy = stats.TrafficLimitStrategy
			result.HwidDeviceLimit = stats.HwidDeviceLimit
			result.ExternalSquadUUID = stats.ExternalSquadUUID
			result.Tag = stats.Tag
			result.UserTraffic = stats.UserTraffic
		}
	}
	return result
}

func (r *Client) patchPanelUser(ctx context.Context, user *PanelUser, fields map[string]any) (*PanelUser, error) {
	if user == nil || (user.ID <= 0 && user.UUID == uuid.Nil) {
		return nil, ErrAdminSubscriptionNotFound
	}

	v3Body := cloneMap(fields)
	v3Body["id"] = user.ID
	var payload struct {
		Response PanelUser `json:"response"`
	}
	err := r.doAPIJSON(ctx, http.MethodPatch, "/api/users", v3Body, &payload)
	if err == nil {
		return &payload.Response, nil
	}
	if user.UUID == uuid.Nil || !isLegacyFallbackError(err) {
		return nil, err
	}

	legacyBody := cloneMap(fields)
	legacyBody["uuid"] = user.UUID.String()
	if legacyErr := r.doAPIJSON(ctx, http.MethodPatch, "/api/users", legacyBody, &payload); legacyErr != nil {
		return nil, errors.Join(err, legacyErr)
	}
	return &payload.Response, nil
}

func (r *Client) createPanelUser(ctx context.Context, fields map[string]any) (*PanelUser, error) {
	var payload struct {
		Response PanelUser `json:"response"`
	}
	err := r.doAPIJSON(ctx, http.MethodPost, "/api/users", fields, &payload)
	if err == nil {
		return &payload.Response, nil
	}
	if !isLegacyFallbackError(err) {
		return nil, err
	}

	legacyBody := cloneMap(fields)
	legacyBody["uuid"] = uuid.New().String()
	if legacyErr := r.doAPIJSON(ctx, http.MethodPost, "/api/users", legacyBody, &payload); legacyErr != nil {
		return nil, errors.Join(err, legacyErr)
	}
	return &payload.Response, nil
}

func (r *Client) getPanelUserByIdentity(ctx context.Context, userID int64, userUUID uuid.UUID) (*PanelUser, error) {
	if userID <= 0 && userUUID == uuid.Nil {
		return nil, ErrAdminSubscriptionNotFound
	}
	identifier := strconv.FormatInt(userID, 10)
	if userID <= 0 {
		identifier = userUUID.String()
	}
	var payload struct {
		Response PanelUser `json:"response"`
	}
	err := r.doAPIJSON(ctx, http.MethodGet, "/api/users/"+identifier, nil, &payload)
	if err == nil {
		return &payload.Response, nil
	}
	if !isLegacyFallbackError(err) {
		return nil, err
	}
	users, legacyErr := r.legacyUsers(ctx)
	if legacyErr != nil {
		return nil, errors.Join(err, legacyErr)
	}
	user := findPanelUser(users, userID, userUUID)
	if user == nil {
		return nil, ErrAdminSubscriptionNotFound
	}
	return user, nil
}

func (r *Client) deletePanelUser(ctx context.Context, user *PanelUser) error {
	if user == nil || (user.ID <= 0 && user.UUID == uuid.Nil) {
		return ErrAdminSubscriptionNotFound
	}
	identifier := strconv.FormatInt(user.ID, 10)
	if user.ID <= 0 {
		identifier = user.UUID.String()
	}
	err := r.doAPIJSON(ctx, http.MethodDelete, "/api/users/"+identifier, nil, nil)
	if err == nil {
		return nil
	}
	if user.ID <= 0 || user.UUID == uuid.Nil || !isLegacyFallbackError(err) {
		return err
	}
	if legacyErr := r.doAPIJSON(ctx, http.MethodDelete, "/api/users/"+user.UUID.String(), nil, nil); legacyErr != nil {
		return errors.Join(err, legacyErr)
	}
	return nil
}

func cloneMap(values map[string]any) map[string]any {
	cloned := make(map[string]any, len(values)+1)
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func findPanelUserByIDOrUsername(users []PanelUser, query string) *PanelUser {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	requestedID, idErr := strconv.ParseInt(query, 10, 64)
	for i := range users {
		if idErr == nil && users[i].ID == requestedID {
			return &users[i]
		}
		if strings.EqualFold(strings.TrimSpace(users[i].Username), query) {
			return &users[i]
		}
	}
	return nil
}

func findPanelUserBySubscriptionLink(users []PanelUser, subscriptionLink string) *PanelUser {
	subscriptionLink = strings.TrimSpace(subscriptionLink)
	if subscriptionLink == "" {
		return nil
	}
	for i := range users {
		if strings.TrimSpace(users[i].SubscriptionURL) == subscriptionLink {
			return &users[i]
		}
	}
	return nil
}

func findPanelUser(users []PanelUser, userID int64, userUUID uuid.UUID) *PanelUser {
	for i := range users {
		if userID > 0 && users[i].ID == userID {
			return &users[i]
		}
		if userUUID != uuid.Nil && users[i].UUID == userUUID {
			return &users[i]
		}
	}
	return nil
}
