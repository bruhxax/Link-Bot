// Package updater talks to a host service with two fixed operations. The bot
// container never receives a Docker socket, a shell, or the host checkout.
package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ContainerDir = "/run/link-bot-updater"

type State struct {
	Available  bool   `json:"available"`
	State      string `json:"state"`
	Stage      string `json:"stage,omitempty"`
	Message    string `json:"message,omitempty"`
	ID         string `json:"id,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Version    string `json:"version,omitempty"`
	Commit     string `json:"commit,omitempty"`
}

type Client struct{ Dir string }

func (c Client) Request(ctx context.Context, start bool) (State, error) {
	dir := c.Dir
	if dir == "" {
		dir = ContainerDir
	}
	key, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		return State{State: "unavailable"}, errors.New("Для обновления из кабинета выполните один раз bash ./update.sh на сервере")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "agent.sock"))
	}}
	defer transport.CloseIdleConnections()
	method, path := "GET", "/status"
	if start {
		method, path = "POST", "/update"
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://updater"+path, bytes.NewReader(nil))
	if err != nil {
		return State{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(key)))
	resp, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return State{State: "unavailable"}, errors.New("Сервис обновления на сервере недоступен")
	}
	defer resp.Body.Close()
	var state State
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&state); err != nil {
		return State{}, errors.New("Не удалось прочитать состояние обновления")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return state, errors.New("Запуск обновления отклонён сервисом")
	}
	return state, nil
}
