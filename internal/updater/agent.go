package updater

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	Repo, Dir, Token string
	mu               sync.Mutex
	state            State
	run              func(context.Context) error
}

func NewAgent(repo, dir, token string) (*Agent, error) {
	root, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(root) || len(token) < 32 {
		return nil, errors.New("invalid updater configuration")
	}
	a := &Agent{Repo: root, Dir: dir, Token: token, state: State{Available: true, State: "idle"}}
	if raw, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		_ = json.Unmarshal(raw, &a.state)
		a.state.Available = true
		if a.state.State == "running" {
			a.state.State = "failed"
			a.state.Message = "Сервис обновления перезапущен. Проверьте состояние бота перед повтором."
			a.state.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
	a.run = a.execute
	return a, nil
}

func (a *Agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"state":"unauthorized"}`))
		return
	}
	if r.URL.Path == "/status" && r.Method == http.MethodGet {
		a.mu.Lock()
		state := a.state
		a.mu.Unlock()
		_ = json.NewEncoder(w).Encode(state)
		return
	}
	if r.URL.Path != "/update" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.ContentLength != 0 || len(r.URL.RawQuery) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	if a.state.State == "running" {
		state := a.state
		a.mu.Unlock()
		_ = json.NewEncoder(w).Encode(state)
		return
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		a.mu.Unlock()
		w.WriteHeader(500)
		return
	}
	a.state = State{Available: true, State: "running", Stage: "Подготовка", ID: hex.EncodeToString(id[:]), StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := a.saveLocked(); err != nil {
		a.state.State = "failed"
		a.mu.Unlock()
		w.WriteHeader(500)
		_ = json.NewEncoder(w).Encode(State{State: "failed", Message: "Не удалось сохранить состояние обновления"})
		return
	}
	state := a.state
	a.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(state)
	go a.work()
}

func (a *Agent) saveLocked() error {
	raw, err := json.Marshal(a.state)
	if err != nil {
		return err
	}
	file := filepath.Join(a.Dir, "state.json.tmp")
	if err = os.WriteFile(file, raw, 0600); err != nil {
		return err
	}
	return os.Rename(file, filepath.Join(a.Dir, "state.json"))
}

func (a *Agent) work() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	err := a.run(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		a.state.State = "failed"
		a.state.Message = safeUpdateError(err)
	} else {
		a.state.State = "completed"
		a.state.Stage = "Готово"
		a.state.Message = "Бот обновлён и перезапущен"
		a.state.Commit, _ = a.git(ctx, "rev-parse", "--short", "HEAD")
		a.state.Version, _ = a.git(ctx, "describe", "--tags", "--always")
	}
	_ = a.saveLocked()
}

var errLocalChanges = errors.New("local changes")
var errWrongCheckout = errors.New("wrong checkout")

func safeUpdateError(err error) string {
	if errors.Is(err, errLocalChanges) {
		return "В исходниках или индексе Git есть изменения. Проверьте их вручную. Локальные настройки docker-compose.yaml и Caddyfile разрешены."
	}
	if errors.Is(err, errWrongCheckout) {
		return "Обновление доступно для ветки main официального Link-Bot. Проверьте репозиторий на сервере."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Обновление не завершилось за 30 минут. Проверьте журнал сервиса на сервере."
	}
	var commandError *exec.ExitError
	if errors.As(err, &commandError) && (commandError.ExitCode() == 21 || commandError.ExitCode() == 22 || commandError.ExitCode() == 23) {
		return "Обновление остановлено при объединении или проверке настроек. Они сохранены в .git/link-bot-update-backups; бот не перезапускался. Подробности: .update-agent/update.log."
	}
	return "Обновление не завершилось. Подробности записаны в .update-agent/update.log на сервере."
}

func (a *Agent) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = a.Repo
	raw, err := cmd.Output()
	return strings.TrimSpace(string(raw)), err
}

func (a *Agent) execute(ctx context.Context) error {
	branch, err := a.git(ctx, "branch", "--show-current")
	if err != nil || branch != "main" {
		return errWrongCheckout
	}
	origin, err := a.git(ctx, "remote", "get-url", "origin")
	if err != nil || !officialOrigin(origin) {
		return errWrongCheckout
	}
	if err := a.checkLocalChanges(ctx); err != nil {
		return err
	}
	script := filepath.Join(a.Repo, "update.sh")
	if _, err = os.Stat(script); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(a.Dir, "update.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	output := &stageWriter{agent: a, log: log}
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = a.Repo
	configureCommand(cmd)
	// Never accept environment or commands from the HTTP request.
	cmd.Env = append(os.Environ(), "LINK_BOT_UPDATE_VERBOSE=1", "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = output
	cmd.Stderr = output
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func (a *Agent) checkLocalChanges(ctx context.Context) error {
	changes, err := a.git(ctx, "status", "--porcelain", "--untracked-files=no", "--", ".", ":(exclude)docker-compose.yaml", ":(exclude)Caddyfile")
	if err != nil {
		return err
	}
	if changes != "" {
		return errLocalChanges
	}
	if _, err = a.git(ctx, "diff", "--cached", "--quiet"); err != nil {
		return errLocalChanges
	}
	return nil
}

func officialOrigin(origin string) bool {
	switch strings.TrimSuffix(strings.ToLower(strings.TrimSpace(origin)), ".git") {
	case "https://github.com/bruhxax/link-bot", "git@github.com:bruhxax/link-bot", "ssh://git@github.com/bruhxax/link-bot":
		return true
	}
	return false
}

type stageWriter struct {
	agent   *Agent
	log     io.Writer
	mu      sync.Mutex
	pending string
	written int
}

func (w *stageWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	if w.written < 2<<20 {
		remaining := (2 << 20) - w.written
		part := p
		if len(part) > remaining {
			part = part[:remaining]
		}
		_, _ = w.log.Write(part)
		w.written += len(part)
	}
	w.pending += string(p)
	if len(w.pending) > 8192 {
		w.pending = w.pending[len(w.pending)-8192:]
	}
	for {
		index := strings.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := w.pending[:index]
		w.pending = w.pending[index+1:]
		if strings.HasPrefix(line, "[") {
			if end := strings.Index(line, "] "); end > 0 && end < 12 {
				stage := strings.TrimSpace(line[end+2:])
				if !knownStage(stage) {
					continue
				}
				a := w.agent
				a.mu.Lock()
				a.state.Stage = stage
				_ = a.saveLocked()
				a.mu.Unlock()
			}
		}
	}
	return n, nil
}

func (a *Agent) String() string { return fmt.Sprintf("Link-Bot updater (%s)", a.Repo) }

func knownStage(stage string) bool {
	switch stage {
	case "Подготовка базы данных", "Сборка новой версии", "Подключение обновлений из Mini App", "Перезапуск бота", "Обновление веб-сервера", "Запуск веб-сервера", "Проверка HTTPS лендинга", "Проверка HTTPS кабинета", "Проверка HTTPS админки":
		return true
	}
	return false
}
