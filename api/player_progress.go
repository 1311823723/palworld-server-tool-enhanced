package api

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/service"
)

type playerProgressItem struct {
	PlayerUID             string                   `json:"player_uid"`
	Nickname              string                   `json:"nickname"`
	Level                 int32                    `json:"level"`
	Exp                   int64                    `json:"exp"`
	PalCount              int                      `json:"pal_count"`
	IsOnline              bool                     `json:"is_online"`
	CurrentSessionSeconds int64                    `json:"current_session_seconds"`
	TotalOnlineSeconds    int64                    `json:"total_online_seconds"`
	Progress              *database.PlayerProgress `json:"progress"`
}

func listPlayerProgress(c *gin.Context) {
	players, err := service.ListPlayers(database.GetDB())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	items := make([]playerProgressItem, 0, len(players))
	capabilities := make(map[string]bool)
	for _, terse := range players {
		player, getErr := service.GetPlayer(database.GetDB(), terse.PlayerUid)
		if getErr != nil {
			continue
		}
		if player.Progress != nil {
			for key, available := range player.Progress.Capabilities {
				capabilities[key] = capabilities[key] || available
			}
		}
		items = append(items, playerProgressItem{
			PlayerUID:             player.PlayerUid,
			Nickname:              player.Nickname,
			Level:                 player.Level,
			Exp:                   player.Exp,
			PalCount:              len(player.Pals),
			IsOnline:              terse.IsOnline,
			CurrentSessionSeconds: terse.CurrentSessionSeconds,
			TotalOnlineSeconds:    terse.TotalOnlineSeconds,
			Progress:              player.Progress,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Level == items[j].Level {
			return items[i].Nickname < items[j].Nickname
		}
		return items[i].Level > items[j].Level
	})
	snapshotMetadata, _ := service.SnapshotMetadata(database.GetDB())
	c.JSON(http.StatusOK, gin.H{
		"items":        items,
		"capabilities": capabilities,
		"parsed_at":    snapshotMetadata.SnapshotTime,
	})
}
