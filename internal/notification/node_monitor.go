package notification

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

type NodeMonitor struct {
	panel    *remnawave.Client
	store    *database.NodeOutageRepository
	settings *runtimeconfig.Service
	bot      *bot.Bot
	adminID  int64
}

func NewNodeMonitor(panel *remnawave.Client, store *database.NodeOutageRepository, settings *runtimeconfig.Service, telegram *bot.Bot, adminID int64) *NodeMonitor {
	return &NodeMonitor{panel: panel, store: store, settings: settings, bot: telegram, adminID: adminID}
}

func (m *NodeMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		checkCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
		if err := m.check(checkCtx); err != nil {
			slog.Warn("node monitor check failed", "error", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *NodeMonitor) check(ctx context.Context) error {
	nodes, err := m.panel.GetNodesStatus(ctx)
	if err != nil {
		return err
	} // A panel outage is not evidence that its nodes went down.
	settings := m.settings.Snapshot()
	threshold := time.Duration(settings.NodeMonitor.OfflineMinutes) * time.Minute
	for _, node := range nodes {
		if node.UUID == "" || node.IsDisabled {
			continue
		}
		if node.IsOnline {
			if err := m.recoverNode(ctx, node, settings); err != nil {
				slog.Warn("node recovery failed", "node", node.Name, "error", err)
			}
			continue
		}
		outage, err := m.store.Start(ctx, node.UUID, node.Name)
		if err != nil {
			return err
		}
		if time.Since(outage.OfflineSince) < threshold {
			continue
		}
		if settings.Features["hide_inactive_node_hosts"] {
			if err := m.hideHosts(ctx, node, nodes); err != nil {
				slog.Warn("node hosts could not be hidden", "node", node.Name, "error", err)
			}
		} else if err := m.restoreHosts(ctx, node.UUID); err != nil {
			slog.Warn("node hosts could not be restored after feature disabled", "node", node.Name, "error", err)
		}
		if !outage.DownNotified {
			hosts := m.hostNames(ctx, node.UUID)
			if err := m.send(ctx, settings.Content.NodeNotification.DownText, outage.Name, hosts, time.Since(outage.OfflineSince)); err != nil {
				slog.Warn("node down notification failed", "node", node.Name, "error", err)
				continue
			}
			if err := m.store.MarkNotified(ctx, node.UUID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *NodeMonitor) hideHosts(ctx context.Context, node remnawave.NodeStatus, nodes []remnawave.NodeStatus) error {
	hosts, err := m.panel.GetHostsStatus(ctx)
	if err != nil {
		return err
	}
	active := map[string]bool{}
	for _, item := range nodes {
		active[strings.ToLower(item.UUID)] = item.IsOnline
	}
	var toHide []string
	for _, host := range hosts {
		if host.UUID == "" || host.IsDisabled || host.IsHidden || !hostOnlyOnInactiveNodes(host, node.UUID, active) {
			continue
		}
		// Persist ownership before changing the panel, so a restart can restore it.
		if err := m.store.RememberHost(ctx, node.UUID, host.UUID, host.Remark); err != nil {
			return err
		}
		toHide = append(toHide, host.UUID)
	}
	return m.panel.SetHostsDisabled(ctx, toHide, true)
}

func hostOnlyOnInactiveNodes(host remnawave.HostStatus, nodeUUID string, active map[string]bool) bool {
	linked := false
	for _, id := range host.Nodes {
		id = strings.ToLower(id)
		if id == strings.ToLower(nodeUUID) {
			linked = true
		}
		available, known := active[id]
		if !known || available {
			return false
		}
	}
	return linked
}

func (m *NodeMonitor) restoreHosts(ctx context.Context, nodeUUID string) error {
	hosts, err := m.store.HiddenHosts(ctx, nodeUUID)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ids = append(ids, host.UUID)
	}
	if err := m.panel.SetHostsDisabled(ctx, ids, false); err != nil {
		return err
	}
	return m.store.ForgetHosts(ctx, nodeUUID)
}

func (m *NodeMonitor) recoverNode(ctx context.Context, node remnawave.NodeStatus, settings runtimeconfig.Settings) error {
	outage, exists, err := m.store.Get(ctx, node.UUID)
	if err != nil || !exists {
		return err
	}
	hosts, err := m.store.HiddenHosts(ctx, node.UUID)
	if err != nil {
		return err
	}
	if err := m.restoreHosts(ctx, node.UUID); err != nil {
		return err
	}
	if outage.DownNotified {
		names := make([]string, 0, len(hosts))
		for _, host := range hosts {
			names = append(names, host.Remark)
		}
		if len(names) == 0 {
			names = m.hostNames(ctx, node.UUID)
		}
		if err := m.send(ctx, settings.Content.NodeNotification.UpText, node.Name, names, time.Since(outage.OfflineSince)); err != nil {
			return err
		}
	}
	return m.store.Finish(ctx, node.UUID)
}

func (m *NodeMonitor) hostNames(ctx context.Context, nodeUUID string) []string {
	hosts, err := m.panel.GetHostsStatus(ctx)
	if err != nil {
		return nil
	}
	var names []string
	for _, host := range hosts {
		for _, id := range host.Nodes {
			if strings.EqualFold(id, nodeUUID) {
				names = append(names, host.Remark)
				break
			}
		}
	}
	return names
}

func (m *NodeMonitor) send(ctx context.Context, template, node string, hosts []string, duration time.Duration) error {
	if m.adminID <= 0 || m.bot == nil {
		return nil
	}
	for i := range hosts {
		hosts[i] = html.EscapeString(hosts[i])
	}
	hostText := "—"
	if len(hosts) > 0 {
		hostText = strings.Join(hosts, ", ")
	}
	minutes := int(duration.Round(time.Minute).Minutes())
	if minutes < 1 {
		minutes = 1
	}
	text := strings.NewReplacer("{node}", html.EscapeString(node), "{hosts}", hostText, "{duration}", fmt.Sprintf("%d мин", minutes)).Replace(template)
	_, err := m.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: m.adminID, Text: text, ParseMode: models.ParseModeHTML})
	return err
}
