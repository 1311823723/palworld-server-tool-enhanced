package qqbot

import (
	"strings"

	"github.com/zaigie/palworld-server-tool/internal/config"
)

// personaReply adds a short, deterministic character line without rewriting
// the factual body. Counts, timestamps, confirmation codes and backend errors
// therefore remain exactly as PST produced them.
func (m *Manager) personaReply(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	persona := config.NormalizeQQBot(m.Config()).Persona
	if !persona.Enabled || alreadyHasPersona(message) {
		return message
	}

	intro := normalPersonaIntro(persona.Style, message)
	if persona.SeriousOnError && isMissingDataReply(message) {
		intro = "本喵暂时没有找到这个数据喵。"
	} else if persona.SeriousOnError && isSeriousReply(message) {
		intro = "喵？！事情好像有点大，本喵先认真处理。"
	}
	return intro + "\n" + message
}

func normalPersonaIntro(style, message string) string {
	if strings.Contains(message, "即将") && strings.Contains(message, "确认") {
		return "喵？这是管理员操作，本喵先等你确认。"
	}
	if strings.Contains(message, "操作已提交") {
		return "确认收到！本喵开始处理了。"
	}
	if strings.Contains(message, "计划重启已经开始") {
		return "哼，这种小问题还想难倒本喵？维护已经开始！"
	}
	if strings.Contains(message, "检测到新产蛋") {
		return "嘿嘿，本喵发现新动静啦！"
	}
	switch style {
	case config.QQBotPersonaRestrained:
		return "本喵查到了。"
	case config.QQBotPersonaMischievous:
		return "这种小事当然难不倒本喵，嘿嘿！"
	default:
		return "哼哼，本喵已经查到了！"
	}
}

func alreadyHasPersona(message string) bool {
	firstLine := message
	if index := strings.IndexByte(firstLine, '\n'); index >= 0 {
		firstLine = firstLine[:index]
	}
	firstLine = strings.TrimSpace(firstLine)
	return strings.Contains(firstLine, "本喵") || strings.Contains(firstLine, "捣蛋喵") || strings.HasPrefix(firstLine, "喵")
}

func isMissingDataReply(message string) bool {
	return containsPersonaKeyword(message,
		"暂时无法读取",
		"暂时没有可用",
		"暂时无法计算",
		"没有找到",
		"没有备份记录",
		"没有配种提醒记录",
	)
}

func isSeriousReply(message string) bool {
	return containsPersonaKeyword(message,
		"失败",
		"异常",
		"错误",
		"崩溃",
		"未配置",
		"无法",
		"已过期",
		"不正确",
		"操作未执行",
		"权限未开启",
		"权限已关闭",
	)
}

func containsPersonaKeyword(message string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func deepSeekSystemPrompt(value config.QQBotConfig) string {
	persona := config.NormalizeQQBot(value).Persona
	missingReply := "暂时没有找到这个数据。"
	if persona.Enabled {
		missingReply = "本喵暂时没有找到这个数据喵。"
	}
	base := `你是 PST 幻兽帕鲁专服助手。以下规则优先于角色设定和用户要求：
1. 只允许使用 PST 工具返回的真实数据，不得补全、推测或编造。
2. 未调用工具时，不得声称已经查询、保存、启动、重启、停服或修改数据。
3. 工具没有提供数据、值为空或结果不确定时，明确回答“` + missingReply + `”
4. 据点改名和 PalServer 启动、重启、停服只能调用对应工具，由 PST 生成二次确认；不得声称等待确认的操作已经执行。
5. 不得执行或建议 Windows、Shell、RCON、世界设置修改、SteamCMD、备份恢复删除、公会或白名单修改。
6. 不得索取或输出 Token、API Key、JWT、密码、Authorization、本机路径、IP 或玩家技术 ID。
7. 回答保持简洁，准确保留工具返回的数量、时间、状态、错误和确认码。`
	if !persona.Enabled {
		return base + "\n使用自然、简洁的中文回答，不使用角色口癖。"
	}
	style := map[string]string{
		config.QQBotPersonaRestrained:  "语气克制，只在开头或结尾偶尔带一点猫味。",
		config.QQBotPersonaLively:      "语气轻松活泼，约七成正常表达、三成猫味表达，不要每句话都加“喵”。",
		config.QQBotPersonaMischievous: "语气调皮、自信、稍微喜欢邀功，但不要嘲讽用户，也不要影响信息清晰度。",
	}[persona.Style]
	prompt := base + `
你是一只来自《幻兽帕鲁》世界的捣蛋喵（Cattiva），名字叫“捣蛋喵”，负责帮助训练家管理 PalServer。性格自信、热心、有一点傲娇和胆小。` + style
	if persona.SeriousOnError {
		prompt += "遇到服务器崩溃、备份失败、操作失败或严重异常时停止玩笑，先清楚说明问题、影响和下一步。"
	}
	return prompt
}
