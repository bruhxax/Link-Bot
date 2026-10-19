package appupdate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Repository = "bruhxax/Link-Bot"
const ReleasePage = "https://github.com/" + Repository + "/releases/latest"
const API = "https://api.github.com/repos/" + Repository

var CommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:$|[-+])`)

type Status struct {
	State         string `json:"state"`
	Kind          string `json:"kind,omitempty"`
	LatestVersion string `json:"latestVersion,omitempty"`
	LatestCommit  string `json:"latestCommit,omitempty"`
	Summary       string `json:"summary,omitempty"`
	AheadBy       int    `json:"aheadBy,omitempty"`
	URL           string `json:"url,omitempty"`
	CheckedAt     string `json:"checkedAt,omitempty"`
}

func JSON(ctx context.Context, client *http.Client, endpoint string, maxBytes int64, target any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Link-Bot-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBytes)).Decode(target) == nil
}

func Newer(current, latest string) (bool, bool) {
	a, b := versionPattern.FindStringSubmatch(strings.TrimSpace(current)), versionPattern.FindStringSubmatch(strings.TrimSpace(latest))
	if len(a) != 4 || len(b) != 4 {
		return false, false
	}
	for i := 1; i < 4; i++ {
		x, e1 := strconv.ParseUint(a[i], 10, 64)
		y, e2 := strconv.ParseUint(b[i], 10, 64)
		if e1 != nil || e2 != nil {
			return false, false
		}
		if x != y {
			return y > x, true
		}
	}
	return false, true
}

type Comparison struct {
	Status   string `json:"status"`
	AheadBy  int    `json:"ahead_by"`
	BehindBy int    `json:"behind_by"`
}

// Check uses main ancestry as the source of truth when the installed commit is
// known. git describe may name an older tag for a build already containing it.
func Check(ctx context.Context, client *http.Client, version, commit string) Status {
	return CheckAt(ctx, client, API, version, commit)
}

func CheckAt(ctx context.Context, client *http.Client, api, version, commit string) Status {
	result := Status{State: "unknown", URL: ReleasePage, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	var release struct {
		Tag string `json:"tag_name"`
	}
	var comparison Comparison
	var heads []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	var releaseOK, headOK, compareOK bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); releaseOK = JSON(ctx, client, api+"/releases/latest", 32<<10, &release) }()
	if CommitPattern.MatchString(commit) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			headOK = JSON(ctx, client, api+"/commits?sha=main&per_page=1", 64<<10, &heads)
		}()
	}
	wg.Wait()
	result.LatestVersion = strings.TrimSpace(release.Tag)
	if headOK && len(heads) == 1 && CommitPattern.MatchString(heads[0].SHA) {
		result.LatestCommit = heads[0].SHA
		result.Summary = strings.SplitN(heads[0].Commit.Message, "\n", 2)[0]
		if strings.EqualFold(commit, result.LatestCommit) {
			result.State = "current"
			return result
		}
		// Page two retains ancestry/counts but omits the potentially huge file
		// patches returned on page one (including generated JS/CSS bundles).
		compareOK = JSON(ctx, client, api+"/compare/"+commit+"..."+result.LatestCommit+"?per_page=1&page=2", 128<<10, &comparison)
	}
	if compareOK {
		switch comparison.Status {
		case "identical", "behind":
			result.State = "current"
			return result
		case "ahead":
			if comparison.AheadBy > 0 && comparison.BehindBy == 0 {
				result.State = "available"
				result.Kind = "commit"
				result.AheadBy = comparison.AheadBy
				result.URL = "https://github.com/" + Repository + "/compare/" + commit + "...main"
				if newer, ok := Newer(version, result.LatestVersion); releaseOK && ok && newer {
					result.Kind = "release"
					result.URL = ReleasePage
				}
				return result
			}
		}
		// A fork or divergent checkout cannot be fast-forwarded safely.
		return result
	}
	if releaseOK && !CommitPattern.MatchString(commit) {
		if newer, ok := Newer(version, result.LatestVersion); ok {
			if newer {
				result.State = "available"
				result.Kind = "release"
			} else if !CommitPattern.MatchString(commit) {
				result.State = "current"
			}
		}
	}
	return result
}
