package qqbot

import (
	"context"
	"fmt"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"github.com/zaigie/palworld-server-tool/service"
	"go.etcd.io/bbolt"
)

// monitorNotifications observes already-persisted PST state. It does not add
// another game connection and initializes its cursors without replaying old
// events when the bot starts.
func (m *Manager) monitorNotifications(ctx context.Context) {
	var previous supervisor.Status
	if m.process != nil {
		previous = m.process.ProcessStatus()
	}
	lastBackupID := latestBackupID(m.db)
	lastBreedingID := latestBreedingID(m.db)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		settings := m.Config().Notifications
		if m.process != nil {
			current := m.process.ProcessStatus()
			if settings.Enabled && settings.ServerCrash && previous.Running && !current.Running && current.DesiredRunning && current.LastExitAt != nil {
				m.Notify("server-crash:"+current.LastExitAt.UTC().Format(time.RFC3339Nano), fmt.Sprintf("PalServer 意外退出，退出码 %d。崩溃守护将按配置处理。", current.LastExitCode))
			}
			if settings.Enabled && settings.WatchdogRestart && current.RestartCount > previous.RestartCount {
				m.Notify(fmt.Sprintf("watchdog-restart:%d", current.RestartCount), fmt.Sprintf("崩溃守护已自动重启 PalServer（本次运行累计 %d 次）。", current.RestartCount))
			}
			if settings.Enabled && settings.ScheduledRestart && current.LastScheduledRestartAt != nil && (previous.LastScheduledRestartAt == nil || !current.LastScheduledRestartAt.Equal(*previous.LastScheduledRestartAt)) {
				m.Notify("scheduled-restart:"+current.LastScheduledRestartAt.UTC().Format(time.RFC3339Nano), "计划重启已经开始，PST 正在保存世界并平滑重启 PalServer。")
			}
			previous = current
		}
		if backup, found := latestBackup(m.db); found && backup.BackupId != lastBackupID {
			lastBackupID = backup.BackupId
			if settings.Enabled && settings.BackupFailure && (backup.Status == "failed" || backup.Error != "") {
				m.Notify("backup-failed:"+backup.BackupId, "存档备份失败，请登录 PST 查看备份记录和日志。")
			}
		}
		if event, found := latestBreedingEvent(m.db); found && event.EventID != lastBreedingID {
			lastBreedingID = event.EventID
			if settings.Enabled && settings.BreedingReminder {
				name := event.BaseDisplayName
				if name == "" {
					name = "未命名据点"
				}
				m.Notify("breeding:"+event.EventID, fmt.Sprintf("配种提醒：%s 检测到新产蛋，当前数量 %d。", name, event.CurrentCount))
			}
		}
	}
}

func latestBackup(db *bbolt.DB) (database.Backup, bool) {
	// Use the existing service query so ordering and future storage migrations
	// remain centralized.
	items, err := service.ListBackups(db, time.Time{}, time.Time{})
	if err != nil || len(items) == 0 {
		return database.Backup{}, false
	}
	return items[len(items)-1], true
}

func latestBackupID(db *bbolt.DB) string {
	item, found := latestBackup(db)
	if !found {
		return ""
	}
	return item.BackupId
}

func latestBreedingEvent(db *bbolt.DB) (database.BreedingFarmEvent, bool) {
	page, err := service.ListBreedingEvents(db, service.BreedingEventQuery{Page: 1, PageSize: 1})
	if err != nil || len(page.Items) == 0 {
		return database.BreedingFarmEvent{}, false
	}
	return page.Items[0], true
}

func latestBreedingID(db *bbolt.DB) string {
	item, found := latestBreedingEvent(db)
	if !found {
		return ""
	}
	return item.EventID
}
